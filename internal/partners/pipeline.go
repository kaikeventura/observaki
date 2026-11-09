// Package partners validates the design-partner recruitment pipeline
// (Story 7.1) and the five-criteria selection screen (Story 7.2). The pipeline
// lives in a CSV file so the founder can edit it in any spreadsheet; the
// validator enforces the AC1/AC3 contract and produces a redacted funnel report
// that is safe to commit (no contacts, no e-mails).
package partners

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RequiredColumns are the CSV columns the validator needs, in any order.
// can_share_data and can_weekly_feedback were added by Story 7.2; an existing
// local pipeline.csv needs both columns (yes, no or unknown).
var RequiredColumns = []string{
	"id",
	"company",
	"segment",
	"contact_name",
	"contact_email",
	"contact_channel",
	"stack",
	"uses_otel",
	"uses_kubernetes",
	"volume_events_per_second",
	"can_share_data",
	"can_weekly_feedback",
	"status",
	"first_contact_at",
	"last_contact_at",
	"reply_at",
	"call_at",
	"decision_at",
	"signed_at",
	"next_action",
	"next_action_due",
}

// Status is the funnel stage of a candidate.
type Status string

// Funnel stages, in order. Declined and no_response are terminal.
const (
	StatusSourced       Status = "sourced"
	StatusContacted     Status = "contacted"
	StatusReplied       Status = "replied"
	StatusCallScheduled Status = "call_scheduled"
	StatusCallDone      Status = "call_done"
	StatusSigned        Status = "signed"
	StatusDeclined      Status = "declined"
	StatusNoResponse    Status = "no_response"
)

var validStatuses = map[Status]bool{
	StatusSourced:       true,
	StatusContacted:     true,
	StatusReplied:       true,
	StatusCallScheduled: true,
	StatusCallDone:      true,
	StatusSigned:        true,
	StatusDeclined:      true,
	StatusNoResponse:    true,
}

var (
	validSegments = map[string]bool{
		"startup":       true,
		"scaleup":       true,
		"platform-team": true,
		"other":         true,
	}
	validChannels = map[string]bool{
		"email":     true,
		"linkedin":  true,
		"slack":     true,
		"community": true,
		"referral":  true,
		"other":     true,
	}
	validTriState = map[string]bool{
		"yes":     true,
		"no":      true,
		"planned": true,
		"unknown": true,
	}
	validYesNoUnknown = map[string]bool{
		"yes":     true,
		"no":      true,
		"unknown": true,
	}
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	dateLayout   = "2006-01-02"
)

// Candidate is one row of the pipeline.
type Candidate struct {
	Line                  int
	ID                    string
	Company               string
	Segment               string
	ContactName           string
	ContactEmail          string
	ContactChannel        string
	Stack                 string
	UsesOTel              string
	UsesKubernetes        string
	VolumeEventsPerSecond string
	CanShareData          string
	CanWeeklyFeedback     string
	Status                Status
	FirstContactAt        string
	LastContactAt         string
	ReplyAt               string
	CallAt                string
	DecisionAt            string
	SignedAt              string
	NextAction            string
	NextActionDue         string
}

// Pipeline is a parsed CSV file.
type Pipeline struct {
	Path       string
	Candidates []Candidate
}

// Parse reads the pipeline CSV. A header-only file (the template) is valid.
func Parse(path string, r io.Reader) (*Pipeline, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.TrimSpace(strings.ToLower(name))] = i
	}
	for _, name := range RequiredColumns {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf("%s: missing required column %q", path, name)
		}
	}

	pipeline := &Pipeline{Path: path}
	for line := 2; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		field := func(name string) string {
			i := index[name]
			if i >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[i])
		}
		pipeline.Candidates = append(pipeline.Candidates, Candidate{
			Line:                  line,
			ID:                    field("id"),
			Company:               field("company"),
			Segment:               field("segment"),
			ContactName:           field("contact_name"),
			ContactEmail:          field("contact_email"),
			ContactChannel:        field("contact_channel"),
			Stack:                 field("stack"),
			UsesOTel:              field("uses_otel"),
			UsesKubernetes:        field("uses_kubernetes"),
			VolumeEventsPerSecond: field("volume_events_per_second"),
			CanShareData:          field("can_share_data"),
			CanWeeklyFeedback:     field("can_weekly_feedback"),
			Status:                Status(field("status")),
			FirstContactAt:        field("first_contact_at"),
			LastContactAt:         field("last_contact_at"),
			ReplyAt:               field("reply_at"),
			CallAt:                field("call_at"),
			DecisionAt:            field("decision_at"),
			SignedAt:              field("signed_at"),
			NextAction:            field("next_action"),
			NextActionDue:         field("next_action_due"),
		})
	}
	return pipeline, nil
}

// HasContact reports whether the row carries a usable way to reach the person.
func (c Candidate) HasContact() bool {
	if c.ContactName == "" {
		return false
	}
	if emailPattern.MatchString(c.ContactEmail) {
		return true
	}
	switch c.ContactChannel {
	case "linkedin", "slack", "community", "referral", "other":
		return true
	}
	return false
}

// Qualified applies the ICP filter available before a call (Story 7.1 AC1):
// the team runs OpenTelemetry (or has it planned) and Kubernetes (or has it
// planned), and the row records a volume estimate. The five-criteria screen
// (Story 7.2) is what decides who can actually be signed.
func (c Candidate) Qualified() bool {
	if !c.HasContact() || c.Stack == "" {
		return false
	}
	if c.UsesOTel != "yes" && c.UsesOTel != "planned" {
		return false
	}
	if c.UsesKubernetes != "yes" && c.UsesKubernetes != "planned" {
		return false
	}
	return c.VolumeEventsPerSecond != ""
}

// Validates reports structural problems for one row.
func (c Candidate) Validate() []string {
	return c.ValidateWith(DefaultCriteria())
}

// ValidateWith reports structural problems for one row, using the Story 7.2
// selection thresholds for the signed-partner rule.
func (c Candidate) ValidateWith(criteria Criteria) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if c.ID == "" {
		add("id is empty")
	}
	if c.Company == "" {
		add("company is empty")
	}
	if !validSegments[c.Segment] {
		add("segment %q is not one of startup, scaleup, platform-team, other", c.Segment)
	}
	if c.ContactName == "" {
		add("contact_name is empty (AC1 requires a contact)")
	}
	if !c.HasContact() {
		add("no usable contact: set a valid contact_email or a non-email contact_channel")
	}
	if c.Stack == "" {
		add("stack is empty (AC1 requires the stack)")
	}
	if !validTriState[c.UsesOTel] {
		add("uses_otel %q is not one of yes, no, planned, unknown", c.UsesOTel)
	}
	if !validTriState[c.UsesKubernetes] {
		add("uses_kubernetes %q is not one of yes, no, planned, unknown", c.UsesKubernetes)
	}
	if !validYesNoUnknown[c.CanShareData] {
		add("can_share_data %q is not one of yes, no, unknown", c.CanShareData)
	}
	if !validYesNoUnknown[c.CanWeeklyFeedback] {
		add("can_weekly_feedback %q is not one of yes, no, unknown", c.CanWeeklyFeedback)
	}
	if c.VolumeEventsPerSecond == "" {
		add("volume_events_per_second is empty (AC1 requires the volume; use a number or \"unknown\")")
	} else if c.VolumeEventsPerSecond != "unknown" {
		if n, err := strconv.Atoi(c.VolumeEventsPerSecond); err != nil || n < 0 {
			add("volume_events_per_second %q is not a non-negative integer or \"unknown\"", c.VolumeEventsPerSecond)
		}
	}
	if !validStatuses[c.Status] {
		add("status %q is not a funnel stage (sourced, contacted, replied, call_scheduled, call_done, signed, declined, no_response)", c.Status)
	}
	if !validChannels[c.ContactChannel] {
		add("contact_channel %q is not one of email, linkedin, slack, community, referral, other", c.ContactChannel)
	}
	if c.NextAction == "" {
		add("next_action is empty")
	}

	dates := map[string]string{
		"first_contact_at": c.FirstContactAt,
		"last_contact_at":  c.LastContactAt,
		"reply_at":         c.ReplyAt,
		"call_at":          c.CallAt,
		"decision_at":      c.DecisionAt,
		"signed_at":        c.SignedAt,
		"next_action_due":  c.NextActionDue,
	}
	parsed := make(map[string]time.Time, len(dates))
	for name, value := range dates {
		if value == "" {
			continue
		}
		t, err := time.Parse(dateLayout, value)
		if err != nil {
			add("%s %q is not YYYY-MM-DD", name, value)
			continue
		}
		parsed[name] = t
	}

	// Funnel consistency: a stage is only reachable when its evidence exists.
	if c.Status != StatusSourced && c.FirstContactAt == "" {
		add("status %q requires first_contact_at", c.Status)
	}
	if c.Status == StatusNoResponse && c.LastContactAt == "" {
		add("status \"no_response\" requires last_contact_at")
	}
	if c.ReplyAt != "" && c.Status == StatusContacted {
		add("reply_at is set but status is \"contacted\"; move to \"replied\"")
	}
	switch c.Status {
	case StatusReplied:
		if c.ReplyAt == "" {
			add("status \"replied\" requires reply_at")
		}
	case StatusCallScheduled, StatusCallDone:
		if c.ReplyAt == "" {
			add("status %q requires reply_at", c.Status)
		}
		if c.CallAt == "" {
			add("status %q requires call_at", c.Status)
		}
	case StatusSigned:
		if c.ReplyAt == "" || c.CallAt == "" || c.DecisionAt == "" || c.SignedAt == "" {
			add("status \"signed\" requires reply_at, call_at, decision_at and signed_at")
		}
	case StatusDeclined:
		if c.ReplyAt == "" || c.CallAt == "" || c.DecisionAt == "" {
			add("status \"declined\" requires reply_at, call_at and decision_at")
		}
		if c.SignedAt != "" {
			add("status \"declined\" must not set signed_at")
		}
	case StatusSourced:
		for _, name := range []string{"first_contact_at", "last_contact_at", "reply_at", "call_at", "decision_at", "signed_at"} {
			if dates[name] != "" {
				add("status \"sourced\" must not set %s", name)
			}
		}
	}

	if c.SignedAt != "" && c.Status != StatusSigned {
		add("signed_at is set but status is %q; move to \"signed\"", c.Status)
	}
	if c.Status == StatusSigned {
		if failed := c.Screen(criteria).Failed(); len(failed) > 0 {
			add("status \"signed\" requires the five Story 7.2 criteria; missing: %s", strings.Join(failed, ", "))
		}
	}
	if c.CallAt != "" && c.Status == StatusReplied {
		add("call_at is set but status is \"replied\"; move to \"call_scheduled\" or \"call_done\"")
	}

	order := []string{"first_contact_at", "reply_at", "call_at", "decision_at", "signed_at"}
	var previous time.Time
	var previousName string
	for _, name := range order {
		t, ok := parsed[name]
		if !ok {
			continue
		}
		if !previous.IsZero() && t.Before(previous) {
			add("%s (%s) is before %s (%s)", name, dates[name], previousName, dates[previousName])
		}
		previous, previousName = t, name
	}
	if last, ok := parsed["last_contact_at"]; ok {
		if first, ok := parsed["first_contact_at"]; ok && last.Before(first) {
			add("last_contact_at (%s) is before first_contact_at (%s)", dates["last_contact_at"], dates["first_contact_at"])
		}
	}

	return problems
}

// Stats is the redacted funnel summary.
type Stats struct {
	Total        int            `json:"total"`
	Qualified    int            `json:"qualified"`
	ByStatus     map[string]int `json:"by_status"`
	Contacted    int            `json:"contacted"`
	Replied      int            `json:"replied"`
	CallsDone    int            `json:"calls_done"`
	CallsPending int            `json:"calls_pending"`
	Signed       int            `json:"signed"`
	Declined     int            `json:"declined"`
	NoResponse   int            `json:"no_response"`
	ReplyRate    float64        `json:"reply_rate"`
	CallRate     float64        `json:"call_rate"`
	SignRate     float64        `json:"sign_rate"`
}

// Summarize computes the funnel without exposing any candidate identity.
func Summarize(candidates []Candidate) Stats {
	stats := Stats{ByStatus: map[string]int{}}
	for _, c := range candidates {
		stats.Total++
		if c.Qualified() {
			stats.Qualified++
		}
		stats.ByStatus[string(c.Status)]++
		if c.Status != StatusSourced {
			stats.Contacted++
		}
		if c.ReplyAt != "" {
			stats.Replied++
		}
		switch c.Status {
		case StatusCallDone, StatusSigned, StatusDeclined:
			stats.CallsDone++
		case StatusCallScheduled:
			stats.CallsPending++
		}
		switch c.Status {
		case StatusSigned:
			stats.Signed++
		case StatusDeclined:
			stats.Declined++
		case StatusNoResponse:
			stats.NoResponse++
		}
	}
	if stats.Contacted > 0 {
		stats.ReplyRate = round(float64(stats.Replied)/float64(stats.Contacted), 3)
	}
	if stats.Replied > 0 {
		stats.CallRate = round(float64(stats.CallsDone)/float64(stats.Replied), 3)
	}
	if stats.CallsDone > 0 {
		stats.SignRate = round(float64(stats.Signed)/float64(stats.CallsDone), 3)
	}
	return stats
}

func round(value float64, places int) float64 {
	factor := 1.0
	for i := 0; i < places; i++ {
		factor *= 10
	}
	return float64(int64(value*factor+0.5)) / factor
}

// Targets are the Story 7.1 acceptance thresholds.
type Targets struct {
	MinQualified int
	MinCalls     int
	MinSigned    int
}

// DefaultTargets mirror AC1 (≥ 20 qualified candidates) and AC3 (≥ 5 calls,
// 3 signed partners).
func DefaultTargets() Targets {
	return Targets{MinQualified: 20, MinCalls: 5, MinSigned: 3}
}

// Check is one acceptance criterion verdict.
type Check struct {
	Name    string
	Target  int
	Actual  int
	Passed  bool
	Message string
}

func (c Check) String() string {
	verdict := "FAIL"
	if c.Passed {
		verdict = "PASS"
	}
	return fmt.Sprintf("%s: %s (%d/%d) %s", c.Name, verdict, c.Actual, c.Target, c.Message)
}

// Evaluate applies the targets to a summary.
func Evaluate(stats Stats, targets Targets) []Check {
	candidates := Check{
		Name:   "AC1 qualified candidates",
		Target: targets.MinQualified,
		Actual: stats.Qualified,
		Passed: stats.Qualified >= targets.MinQualified,
	}
	calls := Check{
		Name:   "AC3 calls happened",
		Target: targets.MinCalls,
		Actual: stats.CallsDone,
		Passed: stats.CallsDone >= targets.MinCalls,
	}
	signed := Check{
		Name:   "AC3 partners signed",
		Target: targets.MinSigned,
		Actual: stats.Signed,
		Passed: stats.Signed >= targets.MinSigned,
	}
	return []Check{candidates, calls, signed}
}

// Validate applies row validation and rejects duplicate ids.
func (p *Pipeline) Validate() []string {
	return p.ValidateWith(DefaultCriteria())
}

// ValidateWith applies row validation with explicit selection thresholds and
// rejects duplicate ids.
func (p *Pipeline) ValidateWith(criteria Criteria) []string {
	var problems []string
	seen := map[string]int{}
	for _, c := range p.Candidates {
		for _, problem := range c.ValidateWith(criteria) {
			problems = append(problems, fmt.Sprintf("%s:%d: %s: %s", p.Path, c.Line, c.ID, problem))
		}
		if c.ID == "" {
			continue
		}
		if first, ok := seen[c.ID]; ok {
			problems = append(problems, fmt.Sprintf("%s:%d: %s: duplicate id (first seen on line %d)", p.Path, c.Line, c.ID, first))
			continue
		}
		seen[c.ID] = c.Line
	}
	return problems
}

// Report renders the redacted funnel report (markdown, no PII).
func Report(p *Pipeline, stats Stats, screen ScreenStats, generated time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Design partner pipeline — report\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", generated.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- File: `%s` (rows: %d, qualified: %d)\n", p.Path, stats.Total, stats.Qualified)
	fmt.Fprintf(&b, "- No contacts, e-mails or notes are included; the tracker itself stays out of the repo.\n")
	fmt.Fprintf(&b, "\n## Funnel\n\n| Stage | Candidates |\n|---|---:|\n")
	order := []Status{StatusSourced, StatusContacted, StatusReplied, StatusCallScheduled, StatusCallDone, StatusSigned, StatusDeclined, StatusNoResponse}
	for _, status := range order {
		fmt.Fprintf(&b, "| %s | %d |\n", status, stats.ByStatus[string(status)])
	}
	screenReport(&b, screen)
	fmt.Fprintf(&b, "\n## Rates\n\n")
	fmt.Fprintf(&b, "- Contacted: %d\n- Replied: %d (reply rate %.0f%%)\n- Calls done: %d (call rate %.0f%%)\n- Calls scheduled: %d\n- Signed: %d (sign rate over calls %.0f%%)\n- Declined: %d\n- No response: %d\n",
		stats.Contacted, stats.Replied, stats.ReplyRate*100, stats.CallsDone, stats.CallRate*100, stats.CallsPending, stats.Signed, stats.SignRate*100, stats.Declined, stats.NoResponse)
	return b.String()
}
