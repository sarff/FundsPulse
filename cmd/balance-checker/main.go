package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/sarff/iSlogger"

	"github.com/sarff/FundsPulse/internal/checker"
	"github.com/sarff/FundsPulse/internal/config"
	"github.com/sarff/FundsPulse/internal/history"
	"github.com/sarff/FundsPulse/internal/notify"
	"github.com/sarff/FundsPulse/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	runOnce := flag.Bool("run-once", false, "Run balance check immediately and exit")
	flag.Parse()

	logger, err := iSlogger.New(iSlogger.DefaultConfig().WithAppName("FundsPulse"))
	if err != nil {
		return fmt.Errorf("init logger: %v", err)
	}
	defer logger.Close()

	if err = godotenv.Load(".env"); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Info("loading .env file", "error", err)
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %v", err)
	}

	notifier, err := notify.NewTelegram(os.ExpandEnv(cfg.Telegram.Token))
	if err != nil {
		return fmt.Errorf("init telegram: %v", err)
	}

	client := service.NewClient()
	historyManager := history.NewManager(cfg.DaysForAverage)

	balanceChecker, err := checker.New(cfg, client, historyManager, notifier, logger)
	if err != nil {
		return fmt.Errorf("init checker: %v", err)
	}

	if *runOnce {
		return balanceChecker.RunOnce(context.Background())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := balanceChecker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("scheduler stopped: %v", err)
	}

	if err := logger.Flush(); err != nil {
		log.Printf("failed to flush logs: %v", err)
	}

	return nil
}
