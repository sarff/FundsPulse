package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config models the application configuration file.
type Config struct {
	DaysForAverage  int                   `yaml:"days_for_avg"`
	Schedule        ScheduleConfig        `yaml:"schedule"`
	MinimumDaysLeft float64               `yaml:"minimum_days_left"`
	HistoryDir      string                `yaml:"history_dir"`
	Telegram        TelegramConfig        `yaml:"telegram"`
	Services        []ServiceConfig       `yaml:"services"`
	StaticServices  []StaticServiceConfig `yaml:"static_services"`
}

// ScheduleConfig keeps daily trigger settings.
type ScheduleConfig struct {
	Time     string `yaml:"time"`
	Timezone string `yaml:"timezone"`
}

// TelegramConfig contains required parameters to notify users.
type TelegramConfig struct {
	Token   string  `yaml:"token"`
	ChatIDs []int64 `yaml:"chat_ids"`
}

// ServiceConfig describes how to query and parse service balance.
type ServiceConfig struct {
	Name           string         `yaml:"name"`
	HistoryFile    string         `yaml:"history_file"`
	CurrencySymbol string         `yaml:"currency_symbol"`
	BillingMode    string         `yaml:"billing_mode"`
	Auth           *AuthConfig    `yaml:"auth"`
	Request        RequestConfig  `yaml:"request"`
	Response       ResponseConfig `yaml:"response"`
}

// StaticServiceConfig describes a fixed monthly payment reminder.
type StaticServiceConfig struct {
	Name             string  `yaml:"name"`
	CurrencySymbol   string  `yaml:"currency_symbol"`
	Amount           float64 `yaml:"amount"`
	BillingDay       int     `yaml:"billing_day"`
	NotifyBeforeDays int     `yaml:"notify_before_days"`
	URLPay           string  `yaml:"url_pay"`
	CardPay          string  `yaml:"card_pay"`
}

// AuthConfig specifies optional pre-request authentication flow.
type AuthConfig struct {
	Request   RequestConfig `yaml:"request"`
	TokenPath string        `yaml:"token_path"`
	Header    string        `yaml:"header"`
	Prefix    string        `yaml:"prefix"`
}

// RequestConfig holds HTTP request parameters.
type RequestConfig struct {
	Method         string            `yaml:"method"`
	URL            string            `yaml:"url"`
	Headers        map[string]string `yaml:"headers"`
	Query          map[string]string `yaml:"query"`
	Body           map[string]any    `yaml:"body"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
}

// ResponseConfig defines how to extract balance values.
type ResponseConfig struct {
	BalancePath   string  `yaml:"balance_path"`
	BalanceScale  float64 `yaml:"balance_scale"`
	CurrencyField string  `yaml:"currency_field"`
	Multiple      bool    `yaml:"multiple" default:"false"`
}

// Load parses configuration file and applies defaults.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %v", err)
	}

	if cfg.DaysForAverage <= 0 {
		cfg.DaysForAverage = 7
	}

	if cfg.HistoryDir == "" {
		cfg.HistoryDir = "data"
	}

	if err := cfg.Schedule.validate(); err != nil {
		return nil, err
	}

	if cfg.Telegram.Token == "" {
		return nil, errors.New("telegram token is required")
	}

	if len(cfg.Telegram.ChatIDs) == 0 {
		return nil, errors.New("at least one telegram chat id is required")
	}

	if len(cfg.Services) == 0 && len(cfg.StaticServices) == 0 {
		return nil, errors.New("services list cannot be empty")
	}

	for i := range cfg.Services {
		svc := &cfg.Services[i]
		if err := svc.applyDefaults(cfg.HistoryDir); err != nil {
			return nil, err
		}
	}

	for i := range cfg.StaticServices {
		svc := &cfg.StaticServices[i]
		if err := svc.validate(); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

// CronExpression returns quartz cron trigger expression for the schedule.
func (s ScheduleConfig) CronExpression() (string, error) {
	t, err := time.Parse("15:04", s.Time)
	if err != nil {
		return "", fmt.Errorf("parse schedule.time: %v", err)
	}
	return fmt.Sprintf("0 %d %d * * *", t.Minute(), t.Hour()), nil
}

// Location resolves configured timezone or local timezone.
func (s ScheduleConfig) Location() (*time.Location, error) {
	if s.Timezone == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone: %v", err)
	}
	return loc, nil
}

func (s *ServiceConfig) applyDefaults(historyDir string) error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("service name is required")
	}

	if s.Auth != nil {
		if err := s.Auth.validate(s.Name); err != nil {
			return err
		}
	}

	if s.Request.Method == "" {
		s.Request.Method = "GET"
	}

	if strings.TrimSpace(s.BillingMode) == "" {
		s.BillingMode = "prepaid"
	} else {
		s.BillingMode = strings.ToLower(strings.TrimSpace(s.BillingMode))
	}

	switch s.BillingMode {
	case "prepaid", "postpaid":
	default:
		return fmt.Errorf("service %q: billing_mode must be prepaid or postpaid", s.Name)
	}

	if strings.TrimSpace(s.Request.URL) == "" {
		return fmt.Errorf("service %q: request url is required", s.Name)
	}

	if strings.TrimSpace(s.Response.BalancePath) == "" {
		return fmt.Errorf("service %q: response.balance_path is required", s.Name)
	}

	if s.Response.BalanceScale == 0 {
		s.Response.BalanceScale = 1
	}

	if s.HistoryFile == "" {
		s.HistoryFile = strings.ToLower(sanitizeFileName(s.Name)) + ".json"
	}

	historyDirClean := filepath.Clean(historyDir)
	fullPath := filepath.Clean(filepath.Join(historyDirClean, s.HistoryFile))
	rel, err := filepath.Rel(historyDirClean, fullPath)
	if err != nil {
		return fmt.Errorf("service %q: resolve history file: %v", s.Name, err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("service %q: history file must be inside history_dir", s.Name)
	}

	s.HistoryFile = fullPath
	return nil
}

func sanitizeFileName(input string) string {
	var builder strings.Builder
	for _, r := range input {
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

func (s *StaticServiceConfig) validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("static service name is required")
	}
	if s.Amount <= 0 {
		return fmt.Errorf("static service %q: amount must be positive", s.Name)
	}
	if s.BillingDay < 1 || s.BillingDay > 31 {
		return fmt.Errorf("static service %q: billing_day must be between 1 and 31", s.Name)
	}
	if s.NotifyBeforeDays < 0 {
		return fmt.Errorf("static service %q: notify_before_days must be >= 0", s.Name)
	}
	return nil
}
