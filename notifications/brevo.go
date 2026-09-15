package notifications

import (
	"bytes"
	"context"
	"encoding/base64"
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
	// ErrBrevoAPIKeyMissing is returned when Brevo API key is not provided or set in environment.
	ErrBrevoAPIKeyMissing = errors.New("brevo api key is required")
	// ErrBrevoSenderMissing is returned when Brevo sender email is not provided or set in environment.
	ErrBrevoSenderMissing = errors.New("brevo sender email is required")
)

// BrevoConfig configures the Brevo transactional email sender.
type BrevoConfig struct {
	APIKey      string
	SenderName  string
	SenderEmail string
	BaseURL     string // defaults to "https://api.brevo.com/v3"
	HTTPClient  *http.Client
}

type brevoSender struct {
	cfg BrevoConfig
}

// NewBrevoSender creates a Brevo API v3 transactional email sender.
func NewBrevoSender(cfg BrevoConfig) (Sender, error) {
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("BREVO_API_KEY")
	}
	if cfg.APIKey == "" {
		return nil, ErrBrevoAPIKeyMissing
	}

	if cfg.SenderEmail == "" {
		cfg.SenderEmail = os.Getenv("BREVO_SENDER_EMAIL")
	}
	if cfg.SenderEmail == "" {
		return nil, ErrBrevoSenderMissing
	}

	if cfg.SenderName == "" {
		cfg.SenderName = os.Getenv("BREVO_SENDER_NAME")
	}
	if cfg.SenderName == "" {
		cfg.SenderName = "Notifications"
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.brevo.com/v3"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	return &brevoSender{cfg: cfg}, nil
}

func (s *brevoSender) Channel() Channel {
	return ChannelEmail
}

type brevoEmailRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoEmailSender struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type brevoAttachment struct {
	Name    string `json:"name"`
	Content string `json:"content"` // Base64 encoded string
}

type brevoSendEmailRequest struct {
	Sender      brevoEmailSender      `json:"sender"`
	To          []brevoEmailRecipient `json:"to"`
	Subject     string                `json:"subject"`
	HTMLContent string                `json:"htmlContent,omitempty"`
	TextContent string                `json:"textContent,omitempty"`
	Attachment  []brevoAttachment     `json:"attachment,omitempty"`
}

func (s *brevoSender) Send(ctx context.Context, msg Message) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if len(msg.Recipients) == 0 {
		return ErrEmptyRecipients
	}

	toRecipients := make([]brevoEmailRecipient, 0, len(msg.Recipients))
	for _, r := range msg.Recipients {
		clean := strings.TrimSpace(r)
		if clean != "" {
			toRecipients = append(toRecipients, brevoEmailRecipient{Email: clean})
		}
	}
	if len(toRecipients) == 0 {
		return ErrEmptyRecipients
	}

	htmlContent := msg.HTMLBody
	textContent := msg.Body
	if htmlContent == "" && textContent != "" {
		htmlContent = fmt.Sprintf("<div>%s</div>", strings.ReplaceAll(textContent, "\n", "<br/>"))
	}
	if textContent == "" && htmlContent != "" {
		textContent = htmlContent
	}

	reqPayload := brevoSendEmailRequest{
		Sender: brevoEmailSender{
			Name:  s.cfg.SenderName,
			Email: s.cfg.SenderEmail,
		},
		To:          toRecipients,
		Subject:     msg.Title,
		HTMLContent: htmlContent,
		TextContent: textContent,
	}

	if len(msg.Attachments) > 0 {
		reqPayload.Attachment = make([]brevoAttachment, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			reqPayload.Attachment = append(reqPayload.Attachment, brevoAttachment{
				Name:    att.Filename,
				Content: base64.StdEncoding.EncodeToString(att.Data),
			})
		}
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal brevo request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/smtp/email", s.cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("api-key", s.cfg.APIKey)

	resp, err := s.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send brevo email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("brevo api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
