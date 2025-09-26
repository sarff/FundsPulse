package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *ScheduleConfig) validate() error {
	if strings.TrimSpace(s.Time) == "" {
		return errors.New("schedule time is required (HH:MM)")
	}
	if _, err := time.Parse("15:04", s.Time); err != nil {
		return fmt.Errorf("parse schedule time: %v", err)
	}
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return fmt.Errorf("load timezone: %v", err)
		}
	}
	return nil
}

func (a *AuthConfig) validate(serviceName string) error {
	if strings.TrimSpace(a.TokenPath) == "" {
		return fmt.Errorf("service %q: auth.token_path is required", serviceName)
	}
	if strings.TrimSpace(a.Header) == "" {
		return fmt.Errorf("service %q: auth.header is required", serviceName)
	}
	if strings.TrimSpace(a.Request.URL) == "" {
		return fmt.Errorf("service %q: auth.request.url is required", serviceName)
	}
	if a.Request.Method == "" {
		a.Request.Method = "POST"
	}
	return nil
}
