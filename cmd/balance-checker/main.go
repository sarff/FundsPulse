package main

import (
	"context"
	"errors"
	"flag"
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
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	runOnce := flag.Bool("run-once", false, "Run balance check immediately and exit")
	flag.Parse()

	logger, err := iSlogger.New(iSlogger.DefaultConfig().WithAppName("FundsPulse"))
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer logger.Close()

	if err = godotenv.Load(".env"); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Info("loading .env file", "error", err)
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	notifier, err := notify.NewTelegram(cfg.Telegram.Token)
	if err != nil {
		logger.Error("init telegram", "error", err)
		os.Exit(1)
	}

	client := service.NewClient()
	historyManager := history.NewManager(cfg.DaysForAverage)

	balanceChecker, err := checker.New(cfg, client, historyManager, notifier, logger)
	if err != nil {
		logger.Error("init checker", "error", err)
		os.Exit(1)
	}

	if *runOnce {
		if err := balanceChecker.RunOnce(context.Background()); err != nil {
			logger.Error("run once failed", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := balanceChecker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("scheduler stopped with error", "error", err)
		os.Exit(1)
	}

	if err := logger.Flush(); err != nil {
		log.Printf("failed to flush logs: %v", err)
	}
}
