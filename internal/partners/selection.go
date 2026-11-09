package partners

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultMinVolume is the volume floor of the selection criteria: a partner
// must send at least 10k events/s (Story 7.2, AC1).
const DefaultMinVolume = 10000

// Criteria holds the thresholds of the Story 7.2 selection screen.
type Criteria struct {
	// MinVolume is the minimum declared events/s before a candidate can be
	// signed. The tracker still accepts "unknown" earlier in the funnel.
	MinVolume int
}

// DefaultCriteria mirrors the five acceptance criteria of Story 7.2: OTel,
// Kubernetes, ≥ 10k events/s, data sharing and weekly feedback.
func DefaultCriteria() Criteria {
	return Criteria{MinVolume: DefaultMinVolume}
}

// Screening is the five-criteria verdict for one candidate. It is computed
// from the tracker fields, so the founder cannot sign a partner that fails the
// screen without the validator rejecting the row.
type Screening struct {
	ID             string
	OTel           bool
	Kubernetes     bool
	Volume         bool
	ShareData      bool
	WeeklyFeedback bool
}

// Eligible reports whether the candidate passes all five criteria.
func (s Screening) Eligible() bool {
	return s.OTel && s.Kubernetes && s.Volume && s.ShareData && s.WeeklyFeedback
}

// Failed lists the criteria the candidate does not meet, in a stable order.
func (s Screening) Failed() []string {
	var failed []string
	if !s.OTel {
		failed = append(failed, "uses_otel=yes")
	}
	if !s.Kubernetes {
		failed = append(failed, "uses_kubernetes=yes")
	}
	if !s.Volume {
		failed = append(failed, "volume_events_per_second>=min")
	}
	if !s.ShareData {
		failed = append(failed, "can_share_data=yes")
	}
	if !s.WeeklyFeedback {
		failed = append(failed, "can_weekly_feedback=yes")
	}
	return failed
}

// CriteriaNames are the screen criteria in report order.
var CriteriaNames = []string{
	"uses_otel",
	"uses_kubernetes",
	"volume_events_per_second",
	"can_share_data",
	"can_weekly_feedback",
}

// Screen applies the five selection criteria to one candidate.
func (c Candidate) Screen(criteria Criteria) Screening {
	minimum := criteria.MinVolume
	if minimum <= 0 {
		minimum = DefaultMinVolume
	}
	volume := false
	if n, err := strconv.Atoi(c.VolumeEventsPerSecond); err == nil && n >= minimum {
		volume = true
	}
	return Screening{
		ID:             c.ID,
		OTel:           c.UsesOTel == "yes",
		Kubernetes:     c.UsesKubernetes == "yes",
		Volume:         volume,
		ShareData:      c.CanShareData == "yes",
		WeeklyFeedback: c.CanWeeklyFeedback == "yes",
	}
}

// ScreenStats is the redacted aggregate of the selection screen.
type ScreenStats struct {
	Evaluated   int            `json:"evaluated"`
	Eligible    int            `json:"eligible"`
	Blocked     int            `json:"blocked"`
	MinVolume   int            `json:"min_volume"`
	ByCriterion map[string]int `json:"by_criterion"`
}

// Screened counts the criteria failures across the pipeline. Only id-level and
// aggregate data are produced; no company, contact or note leaves this call.
func Screened(candidates []Candidate, criteria Criteria) ScreenStats {
	if criteria.MinVolume <= 0 {
		criteria.MinVolume = DefaultMinVolume
	}
	stats := ScreenStats{MinVolume: criteria.MinVolume, ByCriterion: map[string]int{}}
	for _, candidate := range candidates {
		screening := candidate.Screen(criteria)
		stats.Evaluated++
		if screening.Eligible() {
			stats.Eligible++
		} else {
			stats.Blocked++
		}
		if !screening.OTel {
			stats.ByCriterion["uses_otel"]++
		}
		if !screening.Kubernetes {
			stats.ByCriterion["uses_kubernetes"]++
		}
		if !screening.Volume {
			stats.ByCriterion["volume_events_per_second"]++
		}
		if !screening.ShareData {
			stats.ByCriterion["can_share_data"]++
		}
		if !screening.WeeklyFeedback {
			stats.ByCriterion["can_weekly_feedback"]++
		}
	}
	return stats
}

// ScreenSummary renders the one-line console summary of the screen.
func ScreenSummary(stats ScreenStats) string {
	var parts []string
	for _, name := range CriteriaNames {
		parts = append(parts, fmt.Sprintf("%s=%d", name, stats.ByCriterion[name]))
	}
	return fmt.Sprintf("screen: evaluated=%d eligible=%d blocked=%d min_volume=%d | failing: %s",
		stats.Evaluated, stats.Eligible, stats.Blocked, stats.MinVolume, strings.Join(parts, " "))
}

// screenReport appends the redacted selection-screen section to the report.
func screenReport(b *strings.Builder, stats ScreenStats) {
	fmt.Fprintf(b, "\n## Selection screen (Story 7.2, AC1)\n\n")
	fmt.Fprintf(b, "- Thresholds: OTel `yes`, Kubernetes `yes`, volume ≥ %d events/s, data sharing `yes`, weekly feedback `yes`.\n",
		stats.MinVolume)
	fmt.Fprintf(b, "- Eligible: %d of %d evaluated (%d blocked).\n", stats.Eligible, stats.Evaluated, stats.Blocked)
	if stats.Evaluated == 0 {
		return
	}
	fmt.Fprintf(b, "\n| Criterion | Candidates failing |\n|---|---:|\n")
	for _, name := range CriteriaNames {
		fmt.Fprintf(b, "| %s | %d |\n", name, stats.ByCriterion[name])
	}
}
