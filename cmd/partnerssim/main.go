// Command partnerssim generates a deterministic synthetic design-partner funnel
// and runs it through the same validator and goal gate as the real pipeline.
//
// It exists so the pipeline can be rehearsed without real candidates. The output
// is explicitly marked as simulated, lives outside the tracker
// (partners/simulation/, git-ignored) and never counts as AC1/AC3 evidence:
// cmd/partnerscheck refuses simulated rows unless -simulation is passed.
//
// Usage:
//
//	go run ./cmd/partnerssim
//	go run ./cmd/partnerssim -candidates 30 -seed 11 -as-of 2026-12-01
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/observaki/observaki/internal/partners"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "partnerssim: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", filepath.Join("partners", "simulation"), "directory for the generated simulation (git-ignored)")
	seed := flag.Int64("seed", 7, "seed of the deterministic funnel")
	candidates := flag.Int("candidates", 20, "size of the synthetic cohort")
	asOfRaw := flag.String("as-of", time.Now().UTC().Format("2006-01-02"), "reference date (YYYY-MM-DD); no event after it")
	flag.Parse()

	asOf, err := time.Parse("2006-01-02", *asOfRaw)
	if err != nil {
		return fmt.Errorf("-as-of %q is not YYYY-MM-DD", *asOfRaw)
	}
	if *candidates < 20 {
		return fmt.Errorf("-candidates must be at least 20 (AC1 asks for 20 qualified candidates), got %d", *candidates)
	}

	generated := partners.Simulate(partners.SimulateOptions{Seed: *seed, Candidates: *candidates, AsOf: asOf})
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", *outDir, err)
	}

	csvPath := filepath.Join(*outDir, "pipeline.csv")
	if err := writeCSV(csvPath, generated); err != nil {
		return err
	}

	// Round-trip through the real parser so the simulation proves the format,
	// not a private shortcut.
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", csvPath, err)
	}
	defer func() { _ = file.Close() }()
	pipeline, err := partners.Parse(csvPath, file)
	if err != nil {
		return err
	}
	if problems := pipeline.ValidateWith(partners.DefaultCriteria()); len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "%s\n", problem)
		}
		return fmt.Errorf("%d validation problem(s) in the simulated funnel", len(problems))
	}

	stats := partners.Summarize(pipeline.Candidates)
	screen := partners.Screened(pipeline.Candidates, partners.DefaultCriteria())
	checks := partners.Evaluate(stats, partners.DefaultTargets())

	reportPath := filepath.Join(*outDir, "report.md")
	report := partners.SimulationBanner + "\n" + partners.Report(pipeline, stats, screen, time.Now())
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	readme := filepath.Join(*outDir, "README.md")
	if err := os.WriteFile(readme, []byte(simulationReadme(*seed, *candidates, asOf)), 0o644); err != nil {
		return fmt.Errorf("write readme: %w", err)
	}

	fmt.Println("SIMULATION — fake candidates, not AC evidence")
	fmt.Printf("file: %s (rows: %d)\n", csvPath, stats.Total)
	fmt.Printf("report: %s\n", reportPath)
	fmt.Printf("funnel: sourced=%d contacted=%d replied=%d call_scheduled=%d call_done=%d signed=%d declined=%d no_response=%d\n",
		stats.ByStatus["sourced"], stats.ByStatus["contacted"], stats.ByStatus["replied"],
		stats.ByStatus["call_scheduled"], stats.ByStatus["call_done"], stats.ByStatus["signed"],
		stats.ByStatus["declined"], stats.ByStatus["no_response"])
	fmt.Println(partners.ScreenSummary(screen))

	failed := 0
	for _, check := range checks {
		fmt.Printf("SIMULATION %s\n", check)
		if !check.Passed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("the simulated funnel missed %d acceptance check(s); raise -candidates or change -seed", failed)
	}
	fmt.Println("simulation passed: the tooling validates, screens and gates a funnel end to end")
	fmt.Println("reminder: AC1/AC3 stay open until 20 real candidates, 5 calls and 3 signed partners exist (P-01)")
	return nil
}

func writeCSV(path string, candidates []partners.Candidate) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	writer := csv.NewWriter(file)
	if err := writer.Write(partners.CSVColumns); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, candidate := range candidates {
		if err := writer.Write(partners.CSVRow(candidate)); err != nil {
			return fmt.Errorf("write %s: %w", candidate.ID, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush %s: %w", path, err)
	}
	return nil
}

func simulationReadme(seed int64, candidates int, asOf time.Time) string {
	var b strings.Builder
	b.WriteString("# Simulação do pipeline de design partners\n\n")
	b.WriteString(partners.SimulationBanner)
	b.WriteString("\nGerado por `make partners-simulate`.\n\n")
	fmt.Fprintf(&b, "- Seed: `%d` · candidatos: `%d` · data de referência: `%s`\n", seed, candidates, asOf.Format("2006-01-02"))
	b.WriteString("- `pipeline.csv`: funil fictício no formato real do tracker (ids `dp-sim-NNN`).\n")
	b.WriteString("- `report.md`: relatório redigido do funil e da triagem, com banner de simulação.\n")
	b.WriteString("\nDiretório **fora do git**: nada aqui é evidência de AC1/AC3.\n")
	b.WriteString("Para o funil real, use `make partners-init` e preencha `partners/pipeline.csv`.\n")
	return b.String()
}
