package partners

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// CSVColumns is the full tracker header, including the descriptive columns the
// validator does not need but the founder fills in (website, source, notes).
var CSVColumns = []string{
	"id",
	"company",
	"segment",
	"website",
	"contact_name",
	"contact_role",
	"contact_email",
	"contact_channel",
	"stack",
	"uses_otel",
	"uses_kubernetes",
	"volume_events_per_second",
	"can_share_data",
	"can_weekly_feedback",
	"source",
	"status",
	"first_contact_at",
	"last_contact_at",
	"reply_at",
	"call_at",
	"decision_at",
	"signed_at",
	"next_action",
	"next_action_due",
	"notes",
}

// SimulationMarkers are the strings that identify a row as fake. Anything that
// carries one of them is rejected by the goal gate unless -simulation is set,
// so a simulated funnel can never be mistaken for AC1/AC3 evidence.
var SimulationMarkers = []string{
	".example.com",
	".example.org",
	".example.net",
	".invalid",
	"fictício",
	"ficticio",
	"simulado",
	"simulação",
	"simulation",
}

// IsSimulated reports whether the row is synthetic data.
func IsSimulated(c Candidate) bool {
	haystack := strings.ToLower(strings.Join([]string{
		c.ID, c.Company, c.Website, c.ContactName, c.ContactEmail, c.ContactRole,
		c.Source, c.Notes,
	}, " "))
	for _, marker := range SimulationMarkers {
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	return false
}

// SimulatedRows returns the ids of the synthetic rows in the pipeline.
func SimulatedRows(candidates []Candidate) []string {
	var ids []string
	for _, c := range candidates {
		if IsSimulated(c) {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// SimulationBanner is prepended to every simulated artifact.
const SimulationBanner = "> **SIMULAÇÃO — dados fictícios.** Este funil foi gerado por `cmd/partnerssim` para exercitar o pipeline.\n> Não é evidência de AC1/AC3, não conta para o G1 e não substitui design partners reais (P-01).\n"

// SimulateOptions controls the synthetic cohort.
type SimulateOptions struct {
	// Seed keeps the generated funnel reproducible.
	Seed int64
	// Candidates is the cohort size; the default is 20.
	Candidates int
	// AsOf is the reference date: no event is dated after it.
	AsOf time.Time
}

// stagePlan is the funnel mix of the simulation: 8 calls done (3 signed, 3
// call_done, 2 scheduled) so the AC3 target is met by construction.
var stagePlan = []Status{
	StatusSigned, StatusSigned, StatusSigned,
	StatusCallDone, StatusCallDone, StatusCallDone,
	StatusCallScheduled, StatusCallScheduled,
	StatusReplied, StatusReplied, StatusReplied, StatusReplied,
	StatusContacted, StatusContacted, StatusContacted,
	StatusSourced, StatusSourced,
	StatusDeclined, StatusDeclined,
	StatusNoResponse,
}

var (
	simSegments = []string{"startup", "scaleup", "platform-team", "startup", "scaleup"}
	simStacks   = []string{
		"Go, Kubernetes, OTel Collector, Datadog",
		"Rust, Kubernetes, OTel Collector, Grafana Cloud",
		"Node.js, EKS, OTel SDK, Elastic",
		"Python, Kubernetes, OTel Collector, New Relic",
		"Java, OpenShift, OTel Collector, Splunk",
	}
	simRoles = []string{"Head of Platform", "Staff SRE", "Engineering Manager", "SRE Manager", "Platform Engineer"}
	simNames = []string{
		"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel",
		"India", "Juliet", "Kilo", "Lima", "Mike", "November", "Oscar", "Papa",
		"Quebec", "Romeo", "Sierra", "Tango",
	}
)

// Simulate builds a deterministic synthetic cohort. Every row is marked as
// simulated (ids dp-sim-NNN, `.invalid` domains and a note), and the mix always
// contains the 8 calls and 3 signatures the gate expects, plus rows that fail
// the five-criteria screen so the negative paths are exercised too.
func Simulate(opts SimulateOptions) []Candidate {
	if opts.Candidates <= 0 {
		opts.Candidates = 20
	}
	if opts.AsOf.IsZero() {
		opts.AsOf = time.Now()
	}
	asOf := time.Date(opts.AsOf.Year(), opts.AsOf.Month(), opts.AsOf.Day(), 0, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(opts.Seed))
	start := asOf.AddDate(0, 0, -56)

	// Volumes chosen to exercise both sides of the 10k/s threshold.
	volumes := map[int]string{
		0: "12000", 1: "25000", 2: "14000", // signed, all eligible
		3: "6000", 4: "18000", 5: "30000", // call_done; the first fails the volume floor
		6: "unknown", 7: "22000", // scheduled; 6 also has Kubernetes planned
		8: "8000", 9: "11000", 10: "14000", 11: "17000",
		12: "26000", 13: "9000", 14: "13000",
		15: "unknown", 16: "7000",
		17: "6000", 18: "18000",
		19: "10000",
	}
	kubernetes := map[int]string{6: "planned", 12: "planned"}

	candidates := make([]Candidate, 0, opts.Candidates)
	for i := 0; i < opts.Candidates; i++ {
		stage := stagePlan[i%len(stagePlan)]
		id := fmt.Sprintf("dp-sim-%03d", i+1)
		name := simNames[i%len(simNames)]
		host := fmt.Sprintf("https://sim-%03d.example.invalid", i+1)

		candidate := Candidate{
			ID:                    id,
			Company:               fmt.Sprintf("Simulado %s (fictício)", name),
			Segment:               simSegments[i%len(simSegments)],
			Website:               host,
			ContactName:           fmt.Sprintf("Pessoa Simulada %03d", i+1),
			ContactRole:           simRoles[i%len(simRoles)],
			ContactEmail:          fmt.Sprintf("pessoa%03d@sim-%03d.example.invalid", i+1, i+1),
			ContactChannel:        "email",
			Stack:                 simStacks[i%len(simStacks)],
			UsesOTel:              "yes",
			UsesKubernetes:        "yes",
			VolumeEventsPerSecond: volumes[i%len(volumes)],
			Source:                "simulação",
			Status:                stage,
			Notes:                 "SIMULAÇÃO — dados fictícios; não é candidato real e não conta para AC1/AC3",
		}
		if k8s, ok := kubernetes[i]; ok {
			candidate.UsesKubernetes = k8s
		}

		// Data sharing and weekly feedback: everyone signed or in a call can,
		// except one declined row (the reason for refusing) and the early
		// funnel where the answer is still unknown.
		switch {
		case stage == StatusSigned:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "yes", "yes"
		case stage == StatusCallDone:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "yes", "yes"
		case stage == StatusDeclined && i%2 == 1:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "no", "no"
		case stage == StatusDeclined:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "yes", "yes"
		case stage == StatusCallScheduled && i%2 == 0:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "unknown", "unknown"
		case stage == StatusReplied && i%2 == 0:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "unknown", "unknown"
		case stage == StatusReplied:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "yes", "yes"
		default:
			candidate.CanShareData, candidate.CanWeeklyFeedback = "unknown", "unknown"
		}

		if stage == StatusSourced {
			candidate.NextAction = "Simulação: enviar primeiro contato"
			candidate.NextActionDue = asOf.AddDate(0, 0, 2).Format(dateLayout)
			candidates = append(candidates, candidate)
			continue
		}

		first := start.AddDate(0, 0, i)
		last := first.AddDate(0, 0, 2+rng.Intn(3))
		candidate.FirstContactAt = first.Format(dateLayout)
		candidate.LastContactAt = last.Format(dateLayout)
		candidate.NextAction = "Simulação: acompanhar o funil"
		candidate.NextActionDue = asOf.AddDate(0, 0, 3).Format(dateLayout)

		if stage == StatusNoResponse {
			candidate.LastContactAt = first.AddDate(0, 0, 14).Format(dateLayout)
			candidate.NextAction = "Simulação: encerrar sem resposta"
			candidates = append(candidates, candidate)
			continue
		}

		if stage == StatusContacted {
			candidate.NextAction = "Simulação: aguardar resposta (follow-up em 5 dias)"
			candidates = append(candidates, candidate)
			continue
		}

		reply := first.AddDate(0, 0, 1+rng.Intn(3))
		candidate.ReplyAt = reply.Format(dateLayout)

		call := reply.AddDate(0, 0, 3+rng.Intn(8))
		if call.After(asOf) {
			call = asOf
		}
		candidate.CallAt = call.Format(dateLayout)
		if stage == StatusReplied {
			// Replied is pre-call: the schedule date is set but nothing happened yet.
			candidate.CallAt = ""
			candidate.NextAction = "Simulação: agendar a call de 30 min"
			candidates = append(candidates, candidate)
			continue
		}

		decision := call.AddDate(0, 0, 1+rng.Intn(3))
		if decision.After(asOf) {
			decision = asOf
		}
		if stage == StatusCallScheduled {
			if candidate.CallAt != "" {
				// Scheduled rows point at a future call, so no decision yet.
				candidate.CallAt = asOf.AddDate(0, 0, 4+rng.Intn(5)).Format(dateLayout)
			}
			candidate.NextAction = "Simulação: fazer a call"
			candidates = append(candidates, candidate)
			continue
		}

		candidate.DecisionAt = decision.Format(dateLayout)
		if stage == StatusSigned {
			signed := decision.AddDate(0, 0, rng.Intn(3))
			if signed.After(asOf) {
				signed = asOf
			}
			candidate.SignedAt = signed.Format(dateLayout)
			// The validator requires decision_at <= signed_at; signing on the
			// same day is allowed.
			candidate.DecisionAt = signed.Format(dateLayout)
			candidate.NextAction = "Simulação: iniciar onboarding"
		}
		if stage == StatusDeclined {
			candidate.NextAction = "Simulação: registrar o motivo da recusa"
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// CSVRow renders a candidate in CSVColumns order.
func CSVRow(c Candidate) []string {
	return []string{
		c.ID, c.Company, c.Segment, c.Website, c.ContactName, c.ContactRole,
		c.ContactEmail, c.ContactChannel, c.Stack, c.UsesOTel, c.UsesKubernetes,
		c.VolumeEventsPerSecond, c.CanShareData, c.CanWeeklyFeedback, c.Source,
		string(c.Status), c.FirstContactAt, c.LastContactAt, c.ReplyAt, c.CallAt,
		c.DecisionAt, c.SignedAt, c.NextAction, c.NextActionDue, c.Notes,
	}
}
