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

	"github.com/observaki/observaki/internal/config"
	"github.com/observaki/observaki/internal/httpserver"
	"github.com/observaki/observaki/internal/version"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "observaki: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_ = usage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version.String())
		return err
	case "serve":
		return serve()
	case "help", "-h", "--help":
		return usage(stdout)
	default:
		_ = usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel.SlogLevel(),
	}))
	logger.Info("starting observaki", "env", cfg.Environment, "web_dir", cfg.WebDir)

	srv := httpserver.New(cfg.HTTPAddr, version.Version, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

func usage(w io.Writer) error {
	_, err := fmt.Fprintf(w, `observaki — OTel-native observability platform

Usage:
  observaki <command>

Commands:
  version   print version, commit and build date
  serve     start the HTTP API (env %s, default %s)
  help      show this help
`, config.EnvHTTPAddr, config.DefaultHTTPAddr)
	return err
}
