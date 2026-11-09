package partners

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixedAsOf() time.Time {
	return time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
}

func TestSimulateIsDeterministic(t *testing.T) {
	first := Simulate(SimulateOptions{Seed: 7, Candidates: 20, AsOf: fixedAsOf()})
	second := Simulate(SimulateOptions{Seed: 7, Candidates: 20, AsOf: fixedAsOf()})
	if len(first) != len(second) {
		t.Fatalf("cohort sizes differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if !reflect.DeepEqual(CSVRow(first[i]), CSVRow(second[i])) {
			t.Fatalf("row %d differs between runs with the same seed", i)
		}
	}

	other := Simulate(SimulateOptions{Seed: 11, Candidates: 20, AsOf: fixedAsOf()})
	if reflect.DeepEqual(CSVRow(first[0]), CSVRow(other[0])) {
		t.Fatal("a different seed should change the generated funnel")
	}
}

func TestSimulatePassesTheRealValidatorAndGate(t *testing.T) {
	candidates := Simulate(SimulateOptions{Seed: 7, Candidates: 20, AsOf: fixedAsOf()})
	pipeline := &Pipeline{Path: "simulation.csv", Candidates: candidates}
	if problems := pipeline.ValidateWith(DefaultCriteria()); len(problems) > 0 {
		t.Fatalf("simulated rows must satisfy the real contract, got %v", problems)
	}

	stats := Summarize(candidates)
	if stats.Qualified != 20 {
		t.Errorf("qualified = %d, want 20", stats.Qualified)
	}
	if stats.CallsDone < 5 {
		t.Errorf("calls done = %d, want at least 5", stats.CallsDone)
	}
	if stats.Signed != 3 {
		t.Errorf("signed = %d, want 3", stats.Signed)
	}
	for _, check := range Evaluate(stats, DefaultTargets()) {
		if !check.Passed {
			t.Errorf("%s", check)
		}
	}

	screen := Screened(candidates, DefaultCriteria())
	if screen.Eligible < 3 {
		t.Errorf("eligible = %d, want at least the 3 signed partners", screen.Eligible)
	}
	for _, candidate := range candidates {
		if candidate.Status == StatusSigned && !candidate.Screen(DefaultCriteria()).Eligible() {
			t.Errorf("%s is signed but fails the screen: %v", candidate.ID, candidate.Screen(DefaultCriteria()).Failed())
		}
	}
	// The negative paths must exist, otherwise the simulation proves nothing.
	if screen.Blocked == 0 || screen.ByCriterion["volume_events_per_second"] == 0 {
		t.Errorf("simulation should include rows that fail the screen: %+v", screen)
	}
}

func TestSimulateScalesAndStaysValid(t *testing.T) {
	for _, size := range []int{20, 24, 40} {
		candidates := Simulate(SimulateOptions{Seed: 3, Candidates: size, AsOf: fixedAsOf()})
		if len(candidates) != size {
			t.Fatalf("cohort size =%d, want %d", len(candidates), size)
		}
		pipeline := &Pipeline{Path: "simulation.csv", Candidates: candidates}
		if problems := pipeline.ValidateWith(DefaultCriteria()); len(problems) > 0 {
			t.Fatalf("size %d produced %d problems: %v", size, len(problems), problems)
		}
		stats := Summarize(candidates)
		if stats.Signed < 3 || stats.CallsDone < 5 {
			t.Errorf("size %d: signed=%d calls=%d, want >=3 and >=5", size, stats.Signed, stats.CallsDone)
		}
	}
}

func TestSimulateNeverDatesTheFuture(t *testing.T) {
	asOf := fixedAsOf()
	for _, candidate := range Simulate(SimulateOptions{Seed: 5, Candidates: 20, AsOf: asOf}) {
		for name, value := range map[string]string{
			"first_contact_at": candidate.FirstContactAt,
			"last_contact_at":  candidate.LastContactAt,
			"reply_at":         candidate.ReplyAt,
			"decision_at":      candidate.DecisionAt,
			"signed_at":        candidate.SignedAt,
		} {
			if value == "" {
				continue
			}
			date, err := time.Parse(dateLayout, value)
			if err != nil {
				t.Fatalf("%s %q is not a date", name, value)
			}
			if date.After(asOf) {
				t.Errorf("%s %s is after the reference date %s", candidate.ID, name, value)
			}
		}
		// Only a scheduled call may sit in the future.
		if candidate.CallAt != "" && candidate.Status != StatusCallScheduled {
			date, _ := time.Parse(dateLayout, candidate.CallAt)
			if date.After(asOf) {
				t.Errorf("%s call_at %s is in the future for status %s", candidate.ID, candidate.CallAt, candidate.Status)
			}
		}
	}
}

func TestEverySimulatedRowIsMarked(t *testing.T) {
	for _, candidate := range Simulate(SimulateOptions{Seed: 7, Candidates: 20, AsOf: fixedAsOf()}) {
		if !IsSimulated(candidate) {
			t.Errorf("%s is not marked as simulated; it could leak into the real gate", candidate.ID)
		}
		if !strings.HasPrefix(candidate.ID, "dp-sim-") {
			t.Errorf("%s should use the dp-sim- prefix", candidate.ID)
		}
		if !strings.HasSuffix(candidate.Website, ".invalid") && !strings.Contains(candidate.Website, ".example.") {
			t.Errorf("%s should use a reserved domain, got %q", candidate.ID, candidate.Website)
		}
	}
}

func TestIsSimulatedDetectsTheVersionedExample(t *testing.T) {
	f, err := os.Open("../../partners/examples/pipeline-example.csv")
	if err != nil {
		t.Fatalf("open example: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	pipeline, err := Parse("partners/examples/pipeline-example.csv", f)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}
	if got := len(SimulatedRows(pipeline.Candidates)); got != len(pipeline.Candidates) {
		t.Fatalf("%d of %d example rows are marked simulated; the CI fixture could be taken for evidence",
			len(pipeline.Candidates)-got, len(pipeline.Candidates))
	}
}

func TestIsSimulatedDoesNotFlagRealLookingRows(t *testing.T) {
	real := Candidate{
		ID:           "dp-001",
		Company:      "Aurora Pagamentos",
		Website:      "https://aurora.com.br",
		ContactName:  "Ana Ribeiro",
		ContactEmail: "ana@aurora.com.br",
		ContactRole:  "Head of Platform",
		Source:       "referral",
		Notes:        "Indicação do time de plataforma; pediu call em novembro",
	}
	if IsSimulated(real) {
		t.Fatalf("a real-looking row must not be flagged as simulated: %q", strings.Join(SimulatedRows([]Candidate{real}), ","))
	}
}

func TestCSVRowMatchesTheHeader(t *testing.T) {
	candidate := Simulate(SimulateOptions{Seed: 7, Candidates: 20, AsOf: fixedAsOf()})[0]
	if got, want := len(CSVRow(candidate)), len(CSVColumns); got != want {
		t.Fatalf("CSVRow has %d fields, header has %d", got, want)
	}
	for _, column := range RequiredColumns {
		found := false
		for _, name := range CSVColumns {
			if name == column {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required column %q is missing from CSVColumns", column)
		}
	}
}
