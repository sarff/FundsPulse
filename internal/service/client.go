package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sarff/gjson"

	"github.com/sarff/FundsPulse/internal/config"
)

// Client calls remote balance APIs.
type Client struct {
	http *resty.Client
}

// BalanceEntry carries a single balance and optional currency decoded from API response.
type BalanceEntry struct {
	Amount   float64
	Currency string
}

// NewClient builds resty-based client with sane defaults.
func NewClient() *Client {
	http := resty.New()
	http.SetTimeout(30 * time.Second)
	http.SetRetryCount(2)
	http.SetRetryWaitTime(2 * time.Second)
	http.SetRetryMaxWaitTime(10 * time.Second)

	return &Client{http: http}
}

// FetchBalance requests service balance entries and optional currency values.
func (c *Client) FetchBalance(ctx context.Context, cfg config.ServiceConfig) ([]BalanceEntry, error) {
	var authToken string
	if cfg.Auth != nil {
		authReq, authCancel := c.prepareRequest(ctx, cfg.Auth.Request)
		if authCancel != nil {
			defer authCancel()
		}

		authMethod := strings.ToUpper(strings.TrimSpace(cfg.Auth.Request.Method))
		if authMethod == "" {
			authMethod = "POST"
		}

		authResp, authErr := authReq.Execute(authMethod, os.ExpandEnv(cfg.Auth.Request.URL))
		if authErr != nil {
			return nil, fmt.Errorf("auth %s: %v", cfg.Name, authErr)
		}

		if !authResp.IsSuccess() {
			return nil, fmt.Errorf("auth %s: unexpected status %d", cfg.Name, authResp.StatusCode())
		}

		tokenValue := gjson.GetBytes(authResp.Body(), cfg.Auth.TokenPath)
		if !tokenValue.Exists() {
			return nil, fmt.Errorf("auth %s: token path %q not found", cfg.Name, cfg.Auth.TokenPath)
		}

		authToken = strings.TrimSpace(tokenValue.String())
		if authToken == "" {
			return nil, fmt.Errorf("auth %s: token is empty", cfg.Name)
		}
	}

	req, cancel := c.prepareRequest(ctx, cfg.Request)
	if cancel != nil {
		defer cancel()
	}

	if cfg.Auth != nil {
		headerName := strings.TrimSpace(cfg.Auth.Header)
		if headerName != "" {
			prefix := os.ExpandEnv(cfg.Auth.Prefix)
			req.SetHeader(headerName, prefix+authToken)
		}
	}

	method := strings.ToUpper(strings.TrimSpace(cfg.Request.Method))
	if method == "" {
		method = "GET"
	}

	resp, err := req.Execute(method, os.ExpandEnv(cfg.Request.URL))
	if err != nil {
		return nil, fmt.Errorf("request %s: %v", cfg.Name, err)
	}

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("request %s: unexpected status %d", cfg.Name, resp.StatusCode())
	}

	payload := resp.Body()
	balanceValue := gjson.GetBytes(payload, cfg.Response.BalancePath)

	if !balanceValue.Exists() {
		return nil, fmt.Errorf("request %s: balance path %q not found", cfg.Name, cfg.Response.BalancePath)
	}

	scale := cfg.Response.BalanceScale
	if scale == 0 {
		scale = 1
	}

	var entries []BalanceEntry
	if cfg.Response.Multiple {
		if !balanceValue.IsArray() {
			return nil, fmt.Errorf("request %s: balance path %q is not an array", cfg.Name, cfg.Response.BalancePath)
		}

		values := balanceValue.Array()
		entries = make([]BalanceEntry, 0, len(values))
		for _, item := range values {
			entries = append(entries, BalanceEntry{Amount: item.Float() * scale})
		}
	} else {
		entries = []BalanceEntry{{Amount: balanceValue.Float() * scale}}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("request %s: no balances found", cfg.Name)
	}

	if cfg.Response.CurrencyField != "" {
		currencyValue := gjson.GetBytes(payload, cfg.Response.CurrencyField)
		if currencyValue.Exists() {
			if cfg.Response.Multiple && currencyValue.IsArray() {
				currencies := currencyValue.Array()
				for i := range entries {
					if i < len(currencies) {
						entries[i].Currency = strings.TrimSpace(currencies[i].String())
					}
				}
			} else {
				currency := strings.TrimSpace(currencyValue.String())
				for i := range entries {
					entries[i].Currency = currency
				}
			}
		}
	}

	return entries, nil
}

func (c *Client) prepareRequest(ctx context.Context, cfg config.RequestConfig) (*resty.Request, context.CancelFunc) {
	req := c.http.R()

	callCtx := ctx
	var cancel context.CancelFunc
	if cfg.TimeoutSeconds > 0 {
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	}
	req.SetContext(callCtx)

	for key, value := range cfg.Headers {
		req.SetHeader(key, os.ExpandEnv(value))
	}

	for key, value := range cfg.Query {
		req.SetQueryParam(key, os.ExpandEnv(value))
	}

	if cfg.Body != nil {
		req.SetBody(expandPlaceholders(cfg.Body))
	}

	return req, cancel
}

func expandPlaceholders(value any) any {
	switch v := value.(type) {
	case map[string]any:
		res := make(map[string]any, len(v))
		for key, item := range v {
			res[key] = expandPlaceholders(item)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			res[i] = expandPlaceholders(item)
		}
		return res
	case string:
		return os.ExpandEnv(v)
	default:
		return value
	}
}
