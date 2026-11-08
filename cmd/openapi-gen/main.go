// Command openapi-gen writes the OpenAPI contract generated from the API
// operation table, and verifies that the checked-in artifact is current
// (AD-26: the contract is generated and compared in CI).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/observaki/observaki/internal/api"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("openapi-gen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("o", "", "write the generated document to this path (default: stdout)")
	check := flags.String("check", "", "exit non-zero when this file differs from the generated document")
	if err := flags.Parse(args); err != nil {
		return err
	}

	generated := api.OpenAPIJSON()
	switch {
	case *check != "":
		existing, err := os.ReadFile(*check)
		if err != nil {
			return fmt.Errorf("read %s: %w", *check, err)
		}
		if !bytes.Equal(existing, generated) {
			return fmt.Errorf("%s is stale; run `make api-gen` and commit the result", *check)
		}
		_, err = fmt.Fprintf(stdout, "openapi: %s matches the generated contract (%s)\n", *check, api.OpenAPIVersion)
		return err
	case *output != "":
		if err := os.WriteFile(*output, generated, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", *output, err)
		}
		_, err := fmt.Fprintf(stdout, "openapi: wrote %s (%s, %d bytes)\n", *output, api.OpenAPIVersion, len(generated))
		return err
	default:
		_, err := stdout.Write(generated)
		return err
	}
}
