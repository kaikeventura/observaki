package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/observaki/observaki/bench/internal/envprofile"
)

func envCommand(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		envUsage(stderr)
		return 1
	}
	subcommand := args[0]
	flags := flag.NewFlagSet("env "+subcommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", filepath.Join(root, "bench", "env", "t1.json"), "T1 profile")
	pricesPath := flags.String("prices", filepath.Join(root, "bench", "env", "prices.aws-us-east-1.json"), "price snapshot")
	hours := flags.Float64("hours", 4, "run duration in hours for the cost estimate")
	jsonOutput := flags.Bool("json", false, "print the estimate as JSON")
	outputPath := flags.String("out", "", "write generated Terraform variables to this path")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "benchctl: env %s does not accept positional arguments: %s\n", subcommand, strings.Join(flags.Args(), " "))
		return 1
	}

	profile, err := envprofile.LoadProfile(*profilePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}

	switch subcommand {
	case "show":
		return envShow(profile, stdout)
	case "check":
		return envCheck(profile, *profilePath, *pricesPath, stdout, stderr)
	case "cost":
		return envCost(profile, *pricesPath, *hours, *jsonOutput, stdout, stderr)
	case "tfvars":
		return envTFVars(profile, *profilePath, *outputPath, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "benchctl: unknown env command %q\n\n", subcommand)
		envUsage(stderr)
		return 1
	}
}

func envUsage(writer io.Writer) {
	_, _ = fmt.Fprint(writer, `benchctl env — inspect and validate the reproducible benchmark environment

Usage:
  bench/run.sh env show [--profile PATH]
  bench/run.sh env check [--profile PATH] [--prices PATH]
  bench/run.sh env cost [--profile PATH] [--prices PATH] [--hours N] [--json]
  bench/run.sh env tfvars [--profile PATH] [--out PATH]

The profile is the source of truth for region, availability zone, node counts,
instance types and data disks. terraform.tfvars keeps only operator settings
(admin CIDR, SSH public key).
`)
}

func envShow(profile *envprofile.Profile, stdout io.Writer) int {
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(writer, "profile\t%s\n", profile.Name)
	_, _ = fmt.Fprintf(writer, "region\t%s / %s\n", profile.Region, profile.AvailabilityZone)
	_, _ = fmt.Fprintf(writer, "workload\t%s events/s, %.1f B/event, %dh retention\n",
		formatCount(profile.Workload.EventsPerSecond), profile.Workload.WeightedBytesPerEvent(), profile.Workload.RetentionHours)
	_, _ = fmt.Fprintln(writer, "role\tcount\trange\tinstance\tvcpu\tmemory\tdata")
	for _, role := range profile.Roles {
		data := "none"
		if role.DataDisks > 0 {
			data = fmt.Sprintf("%d x %d GiB", role.DataDisks, role.DataDiskGiB)
		}
		_, _ = fmt.Fprintf(writer, "%s\t%d\t%d-%d\t%s\t%d\t%d GiB\t%s\n",
			role.Name, role.Count, role.Min, role.Max, role.InstanceType, role.VCPUs, role.MemoryGiB, data)
	}
	if err := writer.Flush(); err != nil {
		return 1
	}
	if len(profile.Software) > 0 {
		keys := make([]string, 0, len(profile.Software))
		for key := range profile.Software {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		_, _ = fmt.Fprintln(stdout, "\nsoftware:")
		for _, key := range keys {
			_, _ = fmt.Fprintf(stdout, "  %s: %s\n", key, profile.Software[key])
		}
	}
	if len(profile.Notes) > 0 {
		_, _ = fmt.Fprintln(stdout, "\nnotes:")
		for _, note := range profile.Notes {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", note)
		}
	}
	return 0
}

func envCheck(profile *envprofile.Profile, profilePath, pricesPath string, stdout, stderr io.Writer) int {
	if problems := profileProblems(profile); len(problems) > 0 {
		for _, problem := range problems {
			_, _ = fmt.Fprintf(stderr, "fail profile: %s\n", problem)
		}
		return 1
	}
	prices, err := envprofile.LoadPrices(pricesPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	if problems := validationProblems(prices.Validate(profile)); len(problems) > 0 {
		for _, problem := range problems {
			_, _ = fmt.Fprintf(stderr, "fail prices: %s\n", problem)
		}
		return 1
	}
	capacity, err := profile.Capacity()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	cost, err := envprofile.Estimate(profile, prices, 4)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}

	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(writer, "ok\tprofile\t%s (%s)\n", profile.Name, profilePath)
	_, _ = fmt.Fprintf(writer, "ok\tnodes\t%s\n", roleCounts(profile))
	_, _ = fmt.Fprintf(writer, "ok\tclickhouse\t%s raw, %s logical, %s required, %+.1f%% headroom\n",
		humanBytes(capacity.ClickHouseRawBytes), humanBytes(capacity.ClickHouseLogicalBytes),
		humanBytes(capacity.ClickHouseRequiredBytes), capacity.ClickHouseHeadroomPercent)
	_, _ = fmt.Fprintf(writer, "ok\tvictoriametrics\t%s raw, %s logical, %s required, %+.1f%% headroom\n",
		humanBytes(capacity.VictoriaMetricsRawBytes), humanBytes(capacity.VictoriaMetricsLogicalBytes),
		humanBytes(capacity.VictoriaMetricsRequiredBytes), capacity.VictoriaMetricsHeadroomPercent)
	_, _ = fmt.Fprintf(writer, "ok\tnats\t%s raw, %s logical, %s required for %dh, %+.1f%% headroom\n",
		humanBytes(capacity.NATSRawBytes), humanBytes(capacity.NATSLogicalBytes),
		humanBytes(capacity.NATSRequiredBytes), profile.Workload.NATSBufferHours, capacity.NATSHeadroomPercent)
	_, _ = fmt.Fprintf(writer, "ok\tgateway\t%s events/s capacity for %s events/s\n",
		formatCount(capacity.GatewayCapacity), formatCount(profile.Workload.EventsPerSecond))
	_, _ = fmt.Fprintf(writer, "ok\tloadgen\t%s events/s capacity for %s events/s\n",
		formatCount(capacity.LoadgenCapacity), formatCount(profile.Workload.EventsPerSecond))
	_, _ = fmt.Fprintf(writer, "ok\tprices\t%s, as of %s\n", prices.Currency, prices.AsOf)
	_, _ = fmt.Fprintf(writer, "ok\tcost\t%s %.2f/hour, %s %.2f/4h, %s %.2f/month\n",
		cost.Currency, cost.HourlyTotal, cost.Currency, cost.RunTotal, cost.Currency, cost.MonthlyTotal)
	if err := writer.Flush(); err != nil {
		return 1
	}
	return 0
}

func envCost(profile *envprofile.Profile, pricesPath string, hours float64, asJSON bool, stdout, stderr io.Writer) int {
	prices, err := envprofile.LoadPrices(pricesPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	cost, err := envprofile.Estimate(profile, prices, hours)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
		return 1
	}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(cost); err != nil {
			_, _ = fmt.Fprintf(stderr, "benchctl: %v\n", err)
			return 1
		}
		return 0
	}

	_, _ = fmt.Fprintf(stdout, "profile: %s\n", profile.Name)
	_, _ = fmt.Fprintf(stdout, "workload: %s events/s, %.1f B/event, %dh retention\n",
		formatCount(profile.Workload.EventsPerSecond), profile.Workload.WeightedBytesPerEvent(), profile.Workload.RetentionHours)
	_, _ = fmt.Fprintf(stdout, "run: %.2f h\n\n", cost.Hours)
	writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "ROLE\tCOUNT\tINSTANCE\tINSTANCE/H\tSTORAGE/H\tTHROUGHPUT/H\tIOPS/H\tIP/H\tTOTAL/H")
	for _, role := range cost.Roles {
		_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\n",
			role.Name, role.Count, role.InstanceType, role.InstanceHourly, role.StorageHourly,
			role.ThroughputHourly, role.IOPSHourly, role.PublicIPv4Hourly, role.HourlyTotal)
	}
	_, _ = fmt.Fprintf(writer, "TOTAL\t\t\t\t\t\t\t\t%.2f\n", cost.HourlyTotal)
	if err := writer.Flush(); err != nil {
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "\nhourly: %s %.2f\n", cost.Currency, cost.HourlyTotal)
	_, _ = fmt.Fprintf(stdout, "run:    %s %.2f (%.2f h)\n", cost.Currency, cost.RunTotal, cost.Hours)
	_, _ = fmt.Fprintf(stdout, "month:  %s %.2f (%.0f h)\n", cost.Currency, cost.MonthlyTotal, prices.HoursPerMonth)
	_, _ = fmt.Fprintf(stdout, "cost per TB/day: %s %.2f (%.2f TB/day)\n", cost.Currency, cost.CostPerTBDay, cost.TerabytesPerDay)
	_, _ = fmt.Fprintln(stdout, "\nassumptions:")
	for _, assumption := range cost.Assumptions {
		_, _ = fmt.Fprintf(stdout, "  - %s\n", assumption)
	}
	return 0
}

func envTFVars(profile *envprofile.Profile, profilePath, outputPath string, stdout, stderr io.Writer) int {
	output, err := profile.TFVars(profilePath)
	if err != nil {
		for _, problem := range validationProblems(err) {
			_, _ = fmt.Fprintf(stderr, "fail profile: %s\n", problem)
		}
		return 1
	}
	if outputPath == "" {
		_, _ = fmt.Fprint(stdout, output)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: create output directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outputPath, []byte(output), 0o644); err != nil {
		_, _ = fmt.Fprintf(stderr, "benchctl: write %s: %v\n", outputPath, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", outputPath)
	return 0
}

func profileProblems(profile *envprofile.Profile) []string {
	return validationProblems(profile.Validate())
}

func validationProblems(err error) []string {
	if err == nil {
		return nil
	}
	var validation *envprofile.ValidationError
	if errors.As(err, &validation) {
		return validation.Problems
	}
	return []string{err.Error()}
}

func roleCounts(profile *envprofile.Profile) string {
	parts := make([]string, 0, len(profile.Roles))
	for _, role := range profile.Roles {
		parts = append(parts, fmt.Sprintf("%d %s", role.Count, role.Name))
	}
	return strings.Join(parts, ", ")
}

func humanBytes(value int64) string {
	const (
		gib = int64(1024 * 1024 * 1024)
		tib = 1024 * gib
	)
	switch {
	case value >= tib:
		return fmt.Sprintf("%.1f TiB", float64(value)/float64(tib))
	case value >= gib:
		return fmt.Sprintf("%.1f GiB", float64(value)/float64(gib))
	default:
		return fmt.Sprintf("%d B", value)
	}
}
