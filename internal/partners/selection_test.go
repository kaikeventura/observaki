package partners

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestScreenRequiresAllFiveCriteria(t *testing.T) {
	base := Candidate{
		ID:                    "dp-100",
		UsesOTel:              "yes",
		UsesKubernetes:        "yes",
		VolumeEventsPerSecond: "12000",
		CanShareData:          "yes",
		CanWeeklyFeedback:     "yes",
	}
	if !base.Screen(DefaultCriteria()).Eligible() {
		t.Fatal("baseline candidate should pass the screen")
	}

	tests := []struct {
		name   string
		mutate func(*Candidate)
		want   string
	}{
		{"no OTel", func(c *Candidate) { c.UsesOTel = "no" }, "uses_otel=yes"},
		{"OTel planned is not enough", func(c *Candidate) { c.UsesOTel = "planned" }, "uses_otel=yes"},
		{"no Kubernetes", func(c *Candidate) { c.UsesKubernetes = "no" }, "uses_kubernetes=yes"},
		{"volume unknown", func(c *Candidate) { c.VolumeEventsPerSecond = "unknown" }, "volume_events_per_second>=min"},
		{"volume too low", func(c *Candidate) { c.VolumeEventsPerSecond = "9999" }, "volume_events_per_second>=min"},
		{"cannot share data", func(c *Candidate) { c.CanShareData = "unknown" }, "can_share_data=yes"},
		{"no weekly feedback", func(c *Candidate) { c.CanWeeklyFeedback = "no" }, "can_weekly_feedback=yes"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			screening := candidate.Screen(DefaultCriteria())
			if screening.Eligible() {
				t.Fatal("candidate should not pass the screen")
			}
			if got := strings.Join(screening.Failed(), ", "); !strings.Contains(got, test.want) {
				t.Fatalf("want %q in failed criteria, got %q", test.want, got)
			}
		})
	}
}

func TestSigningRequiresTheScreen(t *testing.T) {
	const signedRow = "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,signed,2026-11-01,2026-11-06,2026-11-02,2026-11-05,2026-11-06,2026-11-06,Iniciar onboarding,2026-11-10,"
	_, problems := parseCriteriaRow(t, signedRow, "no", "yes")
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, `status "signed" requires the five Story 7.2 criteria`) {
		t.Fatalf("want the signed screen rule, got:\n%s", joined)
	}
	if !strings.Contains(joined, "can_share_data=yes") {
		t.Fatalf("want the failing criterion named, got:\n%s", joined)
	}

	if _, problems := parseCriteriaRow(t, signedRow, "yes", "yes"); len(problems) > 0 {
		t.Fatalf("an eligible signed row must validate, got %v", problems)
	}
}

func TestDeclinedAndCallDoneMayFailTheScreen(t *testing.T) {
	const declinedRow = "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,6000,slack,declined,2026-11-01,2026-11-06,2026-11-02,2026-11-05,2026-11-06,,Registrar motivo,2026-11-10,"
	if _, problems := parseCriteriaRow(t, declinedRow, "no", "no"); len(problems) > 0 {
		t.Fatalf("a declined row may fail the screen, got %v", problems)
	}

	const callDoneRow = "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,6000,slack,call_done,2026-11-01,2026-11-06,2026-11-02,2026-11-05,,,Decidir após o volume,2026-11-10,"
	if _, problems := parseCriteriaRow(t, callDoneRow, "unknown", "unknown"); len(problems) > 0 {
		t.Fatalf("a call_done row may still be under evaluation, got %v", problems)
	}
}

func TestValidateWithHonorsCustomVolumeThreshold(t *testing.T) {
	const signedRow = "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,signed,2026-11-01,2026-11-06,2026-11-02,2026-11-05,2026-11-06,2026-11-06,Iniciar onboarding,2026-11-10,"
	pipeline, err := Parse("test.csv", strings.NewReader(header+signedRow+",yes,yes\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if problems := pipeline.ValidateWith(Criteria{MinVolume: 20000}); len(problems) == 0 {
		t.Fatal("a 20k/s threshold must reject a 12k/s partner")
	}
	if problems := pipeline.ValidateWith(Criteria{MinVolume: 10000}); len(problems) > 0 {
		t.Fatalf("a 10k/s threshold must accept a 12k/s partner, got %v", problems)
	}
}

func TestNewCriteriaColumnsAreRequired(t *testing.T) {
	legacy := "id,company,segment,contact_name,contact_email,contact_channel,stack,uses_otel,uses_kubernetes,volume_events_per_second,status,first_contact_at,last_contact_at,reply_at,call_at,decision_at,signed_at,next_action,next_action_due\n"
	if _, err := Parse("test.csv", strings.NewReader(legacy)); err == nil {
		t.Fatal("a header without the Story 7.2 columns must be rejected")
	} else if !strings.Contains(err.Error(), "can_share_data") {
		t.Fatalf("error should name the missing column, got %v", err)
	}
}

func TestCriteriaRejectsPlanned(t *testing.T) {
	const row = "dp-100,Acme,startup,,Ana,CTO,ana@acme.example,email,\"Go, K8s\",yes,yes,12000,slack,sourced,,,,,,Enviar contato,2026-11-10,"
	if _, problems := parseCriteriaRow(t, row, "planned", "yes"); len(problems) == 0 {
		t.Fatal("can_share_data=planned is not a valid answer")
	}
}

func TestScreenedAggregatesTheExample(t *testing.T) {
	f, err := os.Open("../../partners/examples/pipeline-example.csv")
	if err != nil {
		t.Fatalf("open example: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	pipeline, err := Parse("partners/examples/pipeline-example.csv", f)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}
	screen := Screened(pipeline.Candidates, DefaultCriteria())
	if screen.Evaluated != 20 || screen.Eligible != 9 || screen.Blocked != 11 {
		t.Fatalf("unexpected screen totals: %+v", screen)
	}
	want := map[string]int{
		"uses_otel":                0,
		"uses_kubernetes":          2,
		"volume_events_per_second": 7,
		"can_share_data":           5,
		"can_weekly_feedback":      7,
	}
	for name, expected := range want {
		if screen.ByCriterion[name] != expected {
			t.Errorf("%s failures = %d, want %d", name, screen.ByCriterion[name], expected)
		}
	}
	summary := ScreenSummary(screen)
	if !strings.Contains(summary, "eligible=9") || !strings.Contains(summary, "min_volume=10000") {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestReportIncludesTheScreenWithoutPII(t *testing.T) {
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
		CanShareData:          "yes",
		CanWeeklyFeedback:     "yes",
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
	report := Report(pipeline, Summarize(pipeline.Candidates), Screened(pipeline.Candidates, DefaultCriteria()), time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))

	if !strings.Contains(report, "## Selection screen (Story 7.2, AC1)") {
		t.Errorf("report is missing the selection screen section:\n%s", report)
	}
	if !strings.Contains(report, "Eligible: 1 of 1 evaluated (0 blocked).") {
		t.Errorf("report is missing the eligibility line:\n%s", report)
	}
	for _, forbidden := range []string{"Aurora", "Ana", "ana@", "Kickoff"} {
		if strings.Contains(report, forbidden) {
			t.Errorf("report leaks %q:\n%s", forbidden, report)
		}
	}
}
