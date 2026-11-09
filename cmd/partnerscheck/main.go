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
	allowEmpty := flag.Bool("allow-empty", false, "accept a header-only file (template check, skips targets)")
	flag.Parse()

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

	problems := pipeline.Validate()
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "%s\n", problem)
		}
		return fmt.Errorf("%d validation problem(s) in %s", len(problems), *file)
	}

	stats := partners.Summarize(pipeline.Candidates)
	fmt.Printf("Partners pipeline: %s\n", *file)
	fmt.Printf("rows: %d | qualified: %d | signed: %d | declined: %d | no_response: %d\n",
		stats.Total, stats.Qualified, stats.Signed, stats.Declined, stats.NoResponse)
	fmt.Printf("funnel: sourced=%d contacted=%d replied=%d call_scheduled=%d call_done=%d signed=%d declined=%d no_response=%d\n",
		stats.ByStatus["sourced"], stats.ByStatus["contacted"], stats.ByStatus["replied"],
		stats.ByStatus["call_scheduled"], stats.ByStatus["call_done"], stats.ByStatus["signed"],
		stats.ByStatus["declined"], stats.ByStatus["no_response"])
	fmt.Printf("rates: reply %.0f%% | call %.0f%% | sign %.0f%%\n",
		stats.ReplyRate*100, stats.CallRate*100, stats.SignRate*100)

	if *reportPath != "" {
		report := partners.Report(pipeline, stats, time.Now())
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
		fmt.Println(check)
		if !check.Passed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d acceptance check(s) failed", failed)
	}
	return nil
}
