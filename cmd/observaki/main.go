package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/observaki/observaki/internal/httpserver"
	"github.com/observaki/observaki/internal/version"
)

const (
	defaultHTTPAddr = ":8080"
	envHTTPAddr     = "OBSERVAKI_HTTP_ADDR"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "observaki: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version.String())
		return nil
	case "serve":
		return serve()
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve() error {
	addr := os.Getenv(envHTTPAddr)
	if addr == "" {
		addr = defaultHTTPAddr
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv := httpserver.New(addr, version.Version, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `observaki — OTel-native observability platform

Usage:
  observaki <command>

Commands:
  version   print version, commit and build date
  serve     start the HTTP API (env %s, default %s)
  help      show this help
`, envHTTPAddr, defaultHTTPAddr)
}
