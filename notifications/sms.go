// SPDX-License-Identifier: MIT

package notifications

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	// ErrSMSConfigMissing is returned when required SMS gateway configurations are absent.
	ErrSMSConfigMissing = errors.New("sms sender requires endpoint and api key")
)

// SMSConfig configures the transactional SMS gateway adapter (compatible with Indian DLT gateways).
type SMSConfig struct {
	EntityID    string // Principal Entity ID for DLT compliance
	SenderID    string // Approved header/sender ID (e.g. 6-character alphabetic)
	APIKey      string
	Endpoint    string // HTTP gateway URL (e.g. https://api.sms-gateway.com/send)
	HTTPClient  *http.Client
	IsSimulated bool // If true, logs delivery without making external HTTP calls
}

type smsSender struct {
	cfg SMSConfig
}

// NewSMSSender creates a new Sender for delivering transactional SMS notifications.
func NewSMSSender(cfg SMSConfig) (Sender, error) {
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("SMS_API_KEY")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = os.Getenv("SMS_GATEWAY_URL")
	}

	if !cfg.IsSimulated {
		if cfg.APIKey == "" || cfg.Endpoint == "" {
			return nil, ErrSMSConfigMissing
		}
	}

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &smsSender{cfg: cfg}, nil
}

func (s *smsSender) Channel() Channel {
	return ChannelSMS
}

func (s *smsSender) Send(ctx context.Context, msg Message) error {
	if s.cfg.IsSimulated {
		return nil
	}

	for _, recipient := range msg.Recipients {
		cleanPhone := strings.TrimSpace(strings.TrimPrefix(recipient, "+"))
		if cleanPhone == "" {
			continue
		}

		params := url.Values{}
		params.Set("apikey", s.cfg.APIKey)
		params.Set("sender", s.cfg.SenderID)
		params.Set("numbers", cleanPhone)
		params.Set("message", msg.Body)
		if s.cfg.EntityID != "" {
			params.Set("pe_id", s.cfg.EntityID)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, strings.NewReader(params.Encode()))
		if err != nil {
			return fmt.Errorf("failed to construct SMS request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := s.cfg.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("sms gateway request failed: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("sms gateway returned HTTP %d", resp.StatusCode)
		}
	}

	return nil
}
