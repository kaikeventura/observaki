package partners

import (
	"os"
	"strings"
	"testing"
	"time"
)

// header mirrors partners/pipeline.template.csv. The two Story 7.2 columns are
// appended at the end because the parser accepts any column order; parseRow
// fills them with "yes" unless the test overrides them.
const header = "id,company,segment,website,contact_name,contact_role,contact_email,contact_channel,stack,uses_otel,uses_kubernetes,volume_events_per_second,source,status,first_contact_at,last_contact_at,reply_at,call_at,decision_at,signed_at,next_action,next_action_due,notes,can_share_data,can_weekly_feedback\n"

// parse builds a pipeline from a header plus one data row that passes the
// data-sharing criteria.
func parseRow(t *testing.T, row string) (*Pipeline, []string) {
	t.Helper()
	return parseCriteriaRow(t, row, "yes", "yes")
}

// parseCriteriaRow builds a pipeline from one row with explicit Story 7.2
// answers for data sharing and weekly feedback.
func parseCriteriaRow(t *testing.T, row, shareData, weeklyFeedback string) (*Pipeline, []string) {
	t.Helper()
	pipeline, err := Parse("test.csv", strings.NewReader(header+row+","+shareData+","+weeklyFeedback+"\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return pipeline, pipeline.Validate()
}

func TestParseEmbeddedTemplate(t *testing.T) {
	f, err := os.Open("../../partners/pipeline.template.csv")
	if err != nil {
		t.Fatalf("open template: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	pipeline, err := Parse("partners/pipeline.template.csv", f)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	if len(pipeline.Candidates) != 0 {
		t.Fatalf("template should be header-only, got %d rows", len(pipeline.Candidates))
	}
}

func TestExamplePipelineMeetsStory71Targets(t *testing.T) {
	f, err := os.Open("../../partners/examples/pipeline-example.csv")
	if err != nil {
		t.Fatalf("open example: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	pipeline, err := Parse("partners/examples/pipeline-example.csv", f)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}
	if problems := pipeline.Validate(); len(problems) > 0 {
		t.Fatalf("example has %d validation problems: %v", len(problems), problems)
	}

	stats := Summarize(pipeline.Candidates)
	if stats.Total != 20 {
		t.Errorf("total = %d, want 20", stats.Total)
	}
	if stats.Qualified != 20 {
		t.Errorf("qualified = %d, want 20", stats.Qualified)
	}
	for _, check := range Evaluate(stats, DefaultTargets()) {
		if !check.Passed {
			t.Errorf("%s", check)
		}
	}
	if stats.CallsDone != 8 {
		t.Errorf("calls done = %d, want 8", stats.CallsDone)
	}
	if stats.CallsPending != 2 {
		t.Errorf("calls scheduled = %d, want 2", stats.CallsPending)
	}
	if stats.Signed != 3 {
		t.Errorf("signed = %d, want 3", stats.Signed)
	}
	if stats.NoResponse != 1 {
		t.Errorf("no_response = %d, want 1", stats.NoResponse)
	}
}

func TestValidateRow(t *testing.T) {
	tests := []struct {
		name    string
		row     string
		wantErr string
	}{
		{
			name: "minimal sourced row is valid",
			row:  "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,,,,,Enviar contato,2026-11-10,",
		},
		{
			name:    "missing contact",
			row:     "dp-100,Acme,startup,,,CTO,,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,,,,,Enviar contato,2026-11-10,",
			wantErr: "contact_name is empty",
		},
		{
			name:    "invalid email without another channel",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana-at-acme,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,,,,,Enviar contato,2026-11-10,",
			wantErr: "no usable contact",
		},
		{
			name: "invalid email but linkedin channel is enough",
			row:  "dp-100,Acme,startup,,Ana,CTO,ana-at-acme,linkedin,\"Go, K8s\",yes,yes,12000,linkedin,sourced,,,,,,,Enviar contato,2026-11-10,",
		},
		{
			name:    "unknown status",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,interested,,,,,,,Enviar contato,2026-11-10,",
			wantErr: "is not a funnel stage",
		},
		{
			name:    "replied without reply_at",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,replied,2026-11-01,2026-11-02,,,,,,,Enviar contato,2026-11-10,",
			wantErr: "requires reply_at",
		},
		{
			name:    "signed without signed_at",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,signed,2026-11-01,2026-11-02,2026-11-02,2026-11-05,2026-11-06,,Enviar contato,2026-11-10,",
			wantErr: "requires reply_at, call_at, decision_at and signed_at",
		},
		{
			name:    "sourced row with a reply is inconsistent",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,2026-11-02,,,,Enviar contato,2026-11-10,",
			wantErr: "status \"sourced\" must not set reply_at",
		},
		{
			name:    "call before reply",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,call_done,2026-11-01,2026-11-05,2026-11-05,2026-11-03,,,Enviar contato,2026-11-10,",
			wantErr: "call_at (2026-11-03) is before reply_at",
		},
		{
			name:    "bad volume",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,muitos,slack,sourced,,,,,,,Enviar contato,2026-11-10,",
			wantErr: "is not a non-negative integer",
		},
		{
			name:    "bad date",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,,,,,Enviar contato,10/11/2026,",
			wantErr: "is not YYYY-MM-DD",
		},
		{
			name:    "no_response without last contact",
			row:     "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,no_response,2026-11-01,,,,,,Enviar contato,2026-11-10,",
			wantErr: "requires last_contact_at",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, problems := parseRow(t, test.row)
			if test.wantErr == "" {
				if len(problems) > 0 {
					t.Fatalf("unexpected problems: %v", problems)
				}
				return
			}
			joined := strings.Join(problems, "\n")
			if !strings.Contains(joined, test.wantErr) {
				t.Fatalf("want %q in problems, got:\n%s", test.wantErr, joined)
			}
		})
	}
}

func TestDuplicateIDsAreRejected(t *testing.T) {
	rows := "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,,,,,Enviar contato,2026-11-10,,yes,yes\n" +
		"dp-100,Outra,startup,,Bia,CTO,bia@outra.example,email,\"Go, K8s\",yes,yes,9000,slack,sourced,,,,,,,Enviar contato,2026-11-10,,yes,yes\n"
	pipeline, err := Parse("test.csv", strings.NewReader(header+rows))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	problems := pipeline.Validate()
	if len(problems) != 1 || !strings.Contains(problems[0], "duplicate id") {
		t.Fatalf("want one duplicate id problem, got %v", problems)
	}
}

func TestQualifiedRequiresICPSignals(t *testing.T) {
	base := Candidate{
		ContactName:           "Ana",
		ContactEmail:          "ana@acme.example",
		ContactChannel:        "email",
		Stack:                 "Go, K8s",
		UsesOTel:              "yes",
		UsesKubernetes:        "yes",
		VolumeEventsPerSecond: "12000",
	}
	if !base.Qualified() {
		t.Fatal("baseline candidate should be qualified")
	}

	tests := []struct {
		name   string
		mutate func(*Candidate)
	}{
		{"no OTel", func(c *Candidate) { c.UsesOTel = "no" }},
		{"no Kubernetes", func(c *Candidate) { c.UsesKubernetes = "no" }},
		{"no contact", func(c *Candidate) { c.ContactEmail = "" }},
		{"no stack", func(c *Candidate) { c.Stack = "" }},
		{"no volume", func(c *Candidate) { c.VolumeEventsPerSecond = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			if candidate.Qualified() {
				t.Fatal("candidate should not be qualified")
			}
		})
	}

	planned := base
	planned.UsesKubernetes = "planned"
	if !planned.Qualified() {
		t.Fatal("planned Kubernetes still qualifies (Story 7.2 screens the details)")
	}
}

func TestSummarizeFunnelAndRates(t *testing.T) {
	candidates := []Candidate{
		{Status: StatusContacted, FirstContactAt: "2026-11-01"},
		{Status: StatusReplied, FirstContactAt: "2026-11-01", ReplyAt: "2026-11-02"},
		{Status: StatusCallDone, FirstContactAt: "2026-11-01", ReplyAt: "2026-11-02", CallAt: "2026-11-05"},
		{Status: StatusSigned, FirstContactAt: "2026-11-01", ReplyAt: "2026-11-02", CallAt: "2026-11-05", DecisionAt: "2026-11-06", SignedAt: "2026-11-07"},
	}
	stats := Summarize(candidates)
	if stats.Contacted != 4 || stats.Replied != 3 || stats.CallsDone != 2 || stats.Signed != 1 {
		t.Fatalf("unexpected funnel: %+v", stats)
	}
	if stats.ReplyRate != 0.75 {
		t.Errorf("reply rate = %v, want 0.75", stats.ReplyRate)
	}
	if stats.CallRate != 0.667 {
		t.Errorf("call rate = %v, want 0.667", stats.CallRate)
	}
	if stats.SignRate != 0.5 {
		t.Errorf("sign rate = %v, want 0.5", stats.SignRate)
	}
}

func TestEvaluateReportsEachTarget(t *testing.T) {
	stats := Stats{Qualified: 19, CallsDone: 4, Signed: 2}
	checks := Evaluate(stats, DefaultTargets())
	if len(checks) != 3 {
		t.Fatalf("want 3 checks, got %d", len(checks))
	}
	for _, check := range checks {
		if check.Passed {
			t.Errorf("%s should fail", check.Name)
		}
		if !strings.Contains(check.String(), "FAIL") {
			t.Errorf("check string should say FAIL: %s", check)
		}
	}
}

func TestReportLeaksNoContacts(t *testing.T) {
	candidate := Candidate{
		ID:                    "dp-001",
		Company:               "Aurora Pagamentos",
		Segment:               "startup",
		ContactName:           "Ana Ribeiro",
		ContactEmail:          "ana@aurora.example.com",
		ContactChannel:        "email",
		Stack:                 "Go, K8s",
		UsesOTel:              "yes",
		UsesKubernetes:        "yes",
		VolumeEventsPerSecond: "12000",
		Status:                StatusSigned,
		FirstContactAt:        "2026-10-06",
		LastContactAt:         "2026-10-16",
		ReplyAt:               "2026-10-07",
		CallAt:                "2026-10-13",
		DecisionAt:            "2026-10-14",
		SignedAt:              "2026-10-16",
		NextAction:            "Kickoff",
		NextActionDue:         "2026-10-20",
	}
	pipeline := &Pipeline{Path: "partners/pipeline.csv", Candidates: []Candidate{candidate}}
	stats := Summarize(pipeline.Candidates)
	report := Report(pipeline, stats, Screened(pipeline.Candidates, DefaultCriteria()), time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))

	for _, forbidden := range []string{"Aurora", "Ana", "ana@", "@aurora", "Kickoff"} {
		if strings.Contains(report, forbidden) {
			t.Errorf("report leaks %q:\n%s", forbidden, report)
		}
	}
	if !strings.Contains(report, "| signed | 1 |") {
		t.Errorf("report is missing the funnel table:\n%s", report)
	}
}
