// Package main is the kinframe entrypoint. The same binary runs as app or worker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rim-buei/kinframe/internal/config"
	"github.com/rim-buei/kinframe/internal/db"
	"github.com/rim-buei/kinframe/internal/httpapi"
	"github.com/rim-buei/kinframe/internal/jobs"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	cfgPath := fs.String("config", "", "optional path to config file (unused in scaffold)")
	_ = cfgPath
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	args := fs.Args()
	cmd := "app"
	if len(args) > 0 {
		cmd = args[0]
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	switch cmd {
	case "app":
		srv := httpapi.NewServer(cfg, database, logger)
		logger.Info("starting app", "addr", cfg.HTTPAddr)
		return srv.ListenAndServe(ctx)
	case "worker":
		w := jobs.NewWorker(cfg, database, logger)
		logger.Info("starting worker")
		return w.Run(ctx)
	case "version":
		fmt.Println("kinframe scaffold")
		return nil
	default:
		return fmt.Errorf("unknown command %q (want app|worker|version)", cmd)
	}
}
