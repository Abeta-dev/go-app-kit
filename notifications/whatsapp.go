// SPDX-License-Identifier: MIT

package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	// ErrWhatsAppTokenMissing is returned when the WhatsApp Cloud API access token is missing.
	ErrWhatsAppTokenMissing = errors.New("whatsapp access token is required")
	// ErrWhatsAppPhoneIDMissing is returned when the WhatsApp phone number ID is missing.
	ErrWhatsAppPhoneIDMissing = errors.New("whatsapp phone number id is required")
)

// WhatsAppConfig configures the Meta WhatsApp Cloud API sender.
type WhatsAppConfig struct {
	AccessToken   string
	PhoneNumberID string
	BaseURL       string // defaults to "https://graph.facebook.com/v20.0"
	HTTPClient    *http.Client
	IsSimulated   bool // If true, logs delivery without making external HTTP calls
}

type whatsAppSender struct {
	cfg WhatsAppConfig
}

// NewWhatsAppSender creates a new Sender for delivering messages over the Meta WhatsApp Cloud API.
func NewWhatsAppSender(cfg WhatsAppConfig) (Sender, error) {
	if cfg.AccessToken == "" {
		cfg.AccessToken = os.Getenv("WHATSAPP_ACCESS_TOKEN")
	}
	if cfg.PhoneNumberID == "" {
		cfg.PhoneNumberID = os.Getenv("WHATSAPP_PHONE_NUMBER_ID")
	}

	if !cfg.IsSimulated {
		if cfg.AccessToken == "" {
			return nil, ErrWhatsAppTokenMissing
		}
		if cfg.PhoneNumberID == "" {
			return nil, ErrWhatsAppPhoneIDMissing
		}
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://graph.facebook.com/v20.0"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &whatsAppSender{cfg: cfg}, nil
}

func (s *whatsAppSender) Channel() Channel {
	return ChannelWhatsApp
}

func (s *whatsAppSender) Send(ctx context.Context, msg Message) error {
	if s.cfg.IsSimulated {
		return nil
	}

	for _, recipient := range msg.Recipients {
		cleanPhone := strings.TrimSpace(strings.TrimPrefix(recipient, "+"))
		if cleanPhone == "" {
			continue
		}

		payload := map[string]interface{}{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                cleanPhone,
			"type":              "text",
			"text": map[string]string{
				"body": msg.Body,
			},
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal whatsapp payload: %w", err)
		}

		url := fmt.Sprintf("%s/%s/messages", strings.TrimSuffix(s.cfg.BaseURL, "/"), s.cfg.PhoneNumberID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to construct whatsapp request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+s.cfg.AccessToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.cfg.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("whatsapp dispatch request failed: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return fmt.Errorf("whatsapp API returned status %d: %s", resp.StatusCode, string(respBody))
		}
		_ = resp.Body.Close()
	}

	return nil
}
