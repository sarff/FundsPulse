package checker

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/reugn/go-quartz/job"
	"github.com/reugn/go-quartz/quartz"
	"github.com/sarff/iSlogger"

	"github.com/sarff/FundsPulse/internal/config"
	"github.com/sarff/FundsPulse/internal/history"
	"github.com/sarff/FundsPulse/internal/notify"
	"github.com/sarff/FundsPulse/internal/service"
)

// Checker coordinates balance polling and notification workflow.
type Checker struct {
	cfg      *config.Config
	client   *service.Client
	history  *history.Manager
	notifier *notify.Telegram
	logger   *iSlogger.Logger
	location *time.Location
}

type balanceReport struct {
	Currency string
	Balance  float64
	Average  float64
	DaysLeft float64
	Warn     bool
}

// New constructs checker instance.
func New(cfg *config.Config, client *service.Client, history *history.Manager, notifier *notify.Telegram, logger *iSlogger.Logger) (*Checker, error) {
	loc, err := cfg.Schedule.Location()
	if err != nil {
		return nil, err
	}
	return &Checker{
		cfg:      cfg,
		client:   client,
		history:  history,
		notifier: notifier,
		logger:   logger,
		location: loc,
	}, nil
}

// Start boots quartz scheduler and blocks until context cancellation.
func (c *Checker) Start(ctx context.Context) error {
	cronExpr, err := c.cfg.Schedule.CronExpression()
	if err != nil {
		return err
	}

	scheduler, err := quartz.NewStdScheduler()
	if err != nil {
		return fmt.Errorf("create scheduler: %v", err)
	}

	scheduler.Start(ctx)

	jobFunc := job.NewFunctionJob(func(ctx context.Context) (any, error) {
		if err := c.RunOnce(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	})

	jobDetail := quartz.NewJobDetail(jobFunc, quartz.NewJobKey("daily_balance"))
	trigger, err := quartz.NewCronTriggerWithLoc(cronExpr, c.location)
	if err != nil {
		return fmt.Errorf("create cron trigger: %v", err)
	}

	if err := scheduler.ScheduleJob(jobDetail, trigger); err != nil {
		return fmt.Errorf("schedule job: %v", err)
	}

	c.logger.Info("Scheduler started", "cron", cronExpr, "location", c.location.String())

	<-ctx.Done()

	c.logger.Info("Stopping scheduler")
	scheduler.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scheduler.Wait(waitCtx)
	return nil
}

// RunOnce performs balance check immediately.
func (c *Checker) RunOnce(ctx context.Context) error {
	now := time.Now().In(c.location)
	var firstErr error

	for _, svc := range c.cfg.Services {
		report, err := c.processService(ctx, svc, now)
		if err != nil {
			c.logger.Error("Service check failed", "service", svc.Name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			failureMsg := fmt.Sprintf("Service: %s\nError: %v", svc.Name, err)
			if notifyErr := c.notifier.Notify(ctx, c.cfg.Telegram.ChatIDs, failureMsg); notifyErr != nil {
				c.logger.Error("Failed to notify about error", "service", svc.Name, "error", notifyErr)
			}
			continue
		}

		if err := c.notifier.Notify(ctx, c.cfg.Telegram.ChatIDs, report); err != nil {
			c.logger.Error("Failed to notify", "service", svc.Name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	for _, svc := range c.cfg.StaticServices {
		message, ok := c.processStaticService(svc, now)
		if !ok {
			continue
		}
		if err := c.notifier.Notify(ctx, c.cfg.Telegram.ChatIDs, message); err != nil {
			c.logger.Error("Failed to notify", "service", svc.Name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

func (c *Checker) processService(ctx context.Context, svc config.ServiceConfig, now time.Time) (string, error) {
	entries, err := c.client.FetchBalance(ctx, svc)
	if err != nil {
		return "", err
	}

	reports := make([]balanceReport, 0, len(entries))
	multiple := len(entries) > 1

	for idx, entry := range entries {
		currency := strings.TrimSpace(entry.Currency)
		if currency == "" {
			currency = svc.CurrencySymbol
		}

		historyPath := svc.HistoryFile
		if multiple {
			historyPath = historyPathForEntry(svc.HistoryFile, idx, currency)
		}

		stats, statsErr := c.history.Update(historyPath, entry.Amount, now)
		if statsErr != nil {
			return "", fmt.Errorf("update history: %v", statsErr)
		}

		avg := stats.Average
		daysLeft := math.Inf(1)
		warn := false

		if svc.BillingMode != "postpaid" {
			if avg > 0 {
				daysLeft = entry.Amount / avg
			}
			warn = daysLeft != math.Inf(1) && daysLeft < c.cfg.MinimumDaysLeft
		}

		reports = append(reports, balanceReport{
			Currency: currency,
			Balance:  entry.Amount,
			Average:  avg,
			DaysLeft: daysLeft,
			Warn:     warn,
		})

		c.logger.Info(
			"Service check entry",
			"service", svc.Name,
			"index", idx,
			"balance", entry.Amount,
			"avg_daily", avg,
			"days_left", daysLeft,
			"currency", currency,
		)
	}

	message := composeMessage(svc.Name, svc.BillingMode, reports)
	c.logger.Info("Service check complete", "service", svc.Name, "entries", len(reports))
	return message, nil
}

func (c *Checker) processStaticService(svc config.StaticServiceConfig, now time.Time) (string, bool) {
	kind, ok := staticServiceNoticeKind(svc, now)
	if !ok {
		return "", false
	}

	message := composeStaticMessage(svc, kind)
	c.logger.Info("Static service reminder", "service", svc.Name, "kind", kind)
	return message, true
}

func composeMessage(serviceName, billingMode string, entries []balanceReport) string {
	var builder strings.Builder
	overallWarn := false
	label := "Balance"
	if billingMode == "postpaid" {
		label = "Debt"
	}

	for i, entry := range entries {
		if entry.Warn {
			overallWarn = true
		}

		suffix := ""
		if overallWarn {
			suffix = " !!!"
		}

		builder.WriteString(fmt.Sprintf("Service: %s%s\n\n", serviceName, suffix))

		if billingMode == "postpaid" {
			entry.Balance = -entry.Balance
		}
		builder.WriteString(fmt.Sprintf("%s: %s\n", label, formatAmount(entry.Balance, entry.Currency)))
		builder.WriteString(fmt.Sprintf("📉 Avg daily: %f\n", entry.Average))
		if billingMode != "postpaid" {
			builder.WriteString(fmt.Sprintf("📆 Enough for: %s", formatDays(entry.DaysLeft)))
		}
		if i < len(entries)-1 {
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}

func formatAmount(value float64, currency string) string {
	if currency != "" {
		switch currency {
		case "$", "€", "£", "¥":
			return fmt.Sprintf("%s%.2f", currency, value)
		default:
			return fmt.Sprintf("%.2f %s", value, currency)
		}
	}
	return fmt.Sprintf("%.2f", value)
}

func formatDays(days float64) string {
	if math.IsInf(days, 1) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f days", days)
}

func staticServiceNoticeKind(svc config.StaticServiceConfig, now time.Time) (string, bool) {
	today := now.Day()
	if today == svc.BillingDay {
		return "🔥 Payment due today 🔥", true
	}

	if svc.NotifyBeforeDays <= 0 {
		return "", false
	}

	notifyDay := svc.BillingDay - svc.NotifyBeforeDays
	if notifyDay < 0 {
		notifyDay += 30
	}

	if notifyDay > 0 && today == notifyDay {
		return fmt.Sprintf("⏰️️️️️️️Payment reminder (📌%d days left)", svc.NotifyBeforeDays), true
	}

	return "", false
}

func composeStaticMessage(svc config.StaticServiceConfig, kind string) string {
	var builder strings.Builder
	builder.WriteString(kind)
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Service: %s\n", svc.Name))
	builder.WriteString(fmt.Sprintf("Amount: %s\n", formatAmount(svc.Amount, svc.CurrencySymbol)))
	builder.WriteString(fmt.Sprintf("Billing day: %d\n", svc.BillingDay))
	if strings.TrimSpace(svc.URLPay) != "" {
		builder.WriteString(fmt.Sprintf("🔗Pay URL: %s\n", svc.URLPay))
	}
	if strings.TrimSpace(svc.CardPay) != "" {
		builder.WriteString(fmt.Sprintf("💳Card: %s\n", svc.CardPay))
	}
	return strings.TrimSpace(builder.String())
}

func historyPathForEntry(base string, index int, currency string) string {
	dir := filepath.Dir(base)
	filename := filepath.Base(base)
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	suffixParts := []string{fmt.Sprintf("%02d", index+1)}
	if sanitized := sanitizeIdentifier(currency); sanitized != "" {
		suffixParts = append(suffixParts, sanitized)
	}

	suffix := strings.Join(suffixParts, "_")
	if ext != "" {
		return filepath.Join(dir, fmt.Sprintf("%s_%s%s", name, suffix, ext))
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s", name, suffix))
}

func sanitizeIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}

	return strings.Trim(builder.String(), "_")
}
