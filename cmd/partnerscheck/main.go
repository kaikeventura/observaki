// Command partnerscheck validates the design-partner recruitment pipeline
// (Story 7.1) and prints a redacted funnel report.
//
// Usage:
//
//	go run ./cmd/partnerscheck -file partners/examples/pipeline-example.csv
//	go run ./cmd/partnerscheck -file partners/pipeline.csv -report partners/pipeline-report.md
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/observaki/observaki/internal/partners"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "partnerscheck: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	file := flag.String("file", "partners/pipeline.csv", "pipeline CSV to validate")
	reportPath := flag.String("report", "", "write a redacted markdown report to this path")
	minQualified := flag.Int("min-candidates", partners.DefaultTargets().MinQualified, "AC1: minimum qualified candidates")
	minCalls := flag.Int("min-calls", partners.DefaultTargets().MinCalls, "AC3: minimum calls that happened")
	minSigned := flag.Int("min-signed", partners.DefaultTargets().MinSigned, "AC3: minimum signed partners")
	minVolume := flag.Int("min-volume", partners.DefaultMinVolume, "Story 7.2: minimum events/s for a signed partner")
	allowEmpty := flag.Bool("allow-empty", false, "accept a header-only file (template check, skips targets)")
	simulation := flag.Bool("simulation", false, "accept synthetic rows produced by cmd/partnerssim (never counts as evidence)")
	flag.Parse()

	criteria := partners.DefaultCriteria()
	criteria.MinVolume = *minVolume

	f, err := os.Open(*file)
	if err != nil {
		hint := ""
		if os.IsNotExist(err) {
			hint = " (create it with: cp partners/pipeline.template.csv partners/pipeline.csv)"
		}
		return fmt.Errorf("open %s: %w%s", *file, err, hint)
	}
	defer func() { _ = f.Close() }()

	pipeline, err := partners.Parse(*file, f)
	if err != nil {
		return err
	}

	problems := pipeline.ValidateWith(criteria)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "%s\n", problem)
		}
		return fmt.Errorf("%d validation problem(s) in %s", len(problems), *file)
	}

	// A simulated funnel can never stand in for the real goal: refuse it on
	// the real gate so a synthetic cohort cannot be committed as evidence.
	if simulated := partners.SimulatedRows(pipeline.Candidates); len(simulated) > 0 && !*simulation {
		return fmt.Errorf("%d simulated row(s) in %s (first: %s); synthetic data is not AC evidence — run the simulation with cmd/partnerssim or pass -simulation explicitly",
			len(simulated), *file, simulated[0])
	}
	if *simulation {
		fmt.Println("SIMULATION MODE — synthetic rows; the checks below prove the tooling, not AC1/AC3")
	}

	stats := partners.Summarize(pipeline.Candidates)
	screen := partners.Screened(pipeline.Candidates, criteria)
	fmt.Printf("Partners pipeline: %s\n", *file)
	fmt.Printf("rows: %d | qualified: %d | signed: %d | declined: %d | no_response: %d\n",
		stats.Total, stats.Qualified, stats.Signed, stats.Declined, stats.NoResponse)
	fmt.Printf("funnel: sourced=%d contacted=%d replied=%d call_scheduled=%d call_done=%d signed=%d declined=%d no_response=%d\n",
		stats.ByStatus["sourced"], stats.ByStatus["contacted"], stats.ByStatus["replied"],
		stats.ByStatus["call_scheduled"], stats.ByStatus["call_done"], stats.ByStatus["signed"],
		stats.ByStatus["declined"], stats.ByStatus["no_response"])
	fmt.Printf("rates: reply %.0f%% | call %.0f%% | sign %.0f%%\n",
		stats.ReplyRate*100, stats.CallRate*100, stats.SignRate*100)
	fmt.Println(partners.ScreenSummary(screen))

	if *reportPath != "" {
		report := partners.Report(pipeline, stats, screen, time.Now())
		if *simulation {
			report = partners.SimulationBanner + "\n" + report
		}
		if err := os.WriteFile(*reportPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %s: %w", *reportPath, err)
		}
		fmt.Printf("report: %s (redacted, safe to commit)\n", *reportPath)
	}

	if stats.Total == 0 {
		if *allowEmpty {
			fmt.Println("empty pipeline accepted (template only)")
			return nil
		}
		return fmt.Errorf("%s has no candidates", *file)
	}

	targets := partners.Targets{MinQualified: *minQualified, MinCalls: *minCalls, MinSigned: *minSigned}
	failed := 0
	for _, check := range partners.Evaluate(stats, targets) {
		if *simulation {
			fmt.Printf("SIMULATION %s\n", check)
		} else {
			fmt.Println(check)
		}
		if !check.Passed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d acceptance check(s) failed", failed)
	}
	return nil
}
