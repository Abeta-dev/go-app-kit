// SPDX-License-Identifier: MIT

package notifications_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abeta-dev/go-app-kit/notifications"
)

func TestSMSSender_Simulated(t *testing.T) {
	sender, err := notifications.NewSMSSender(notifications.SMSConfig{
		IsSimulated: true,
	})
	if err != nil {
		t.Fatalf("expected nil error for simulated SMS sender, got: %v", err)
	}

	if sender.Channel() != notifications.ChannelSMS {
		t.Fatalf("expected ChannelSMS, got: %s", sender.Channel())
	}

	err = sender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "Test OTP is 123456",
	})
	if err != nil {
		t.Fatalf("expected nil error for simulated send: %v", err)
	}
}

func TestSMSSender_LiveHTTP(t *testing.T) {
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender, err := notifications.NewSMSSender(notifications.SMSConfig{
		EntityID:   "DLT123456",
		TemplateID: "TMPL999",
		SenderID:   "TXTSMS",
		APIKey:     "secret-key",
		Endpoint:   server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create SMS sender: %v", err)
	}

	err = sender.Send(context.Background(), notifications.Message{
		Recipients: []string{"   ", "+", "+919876543210"},
		Body:       "Your code is 654321",
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}

	if !strings.Contains(receivedBody, "numbers=919876543210") ||
		!strings.Contains(receivedBody, "pe_id=DLT123456") ||
		!strings.Contains(receivedBody, "template_id=TMPL999") {
		t.Fatalf("unexpected post body: %s", receivedBody)
	}

	// Override template_id via Message.Metadata
	err = sender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "Override template",
		Metadata:   map[string]string{"template_id": "TMPL_OVERRIDE"},
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}
	if !strings.Contains(receivedBody, "template_id=TMPL_OVERRIDE") {
		t.Fatalf("expected template_id override in body: %s", receivedBody)
	}
}

func TestSMSSender_MissingConfigAndErrors(t *testing.T) {
	_, err := notifications.NewSMSSender(notifications.SMSConfig{})
	if err == nil {
		t.Fatal("expected error when endpoint or key is missing and not simulated")
	}

	// 1. Invalid URL error in NewRequestWithContext
	badURLSender, err := notifications.NewSMSSender(notifications.SMSConfig{
		APIKey:   "key",
		Endpoint: "http://\x7f",
	})
	if err != nil {
		t.Fatalf("NewSMSSender failed: %v", err)
	}
	if err := badURLSender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "test",
	}); err == nil {
		t.Fatal("expected error for invalid SMS endpoint URL")
	}

	// 2. HTTP client transport error
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	// Test non-2xx first
	statusSender, _ := notifications.NewSMSSender(notifications.SMSConfig{
		APIKey:   "key",
		Endpoint: errServer.URL,
	})
	if err := statusSender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "test",
	}); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got: %v", err)
	}

	// Close server so HTTPClient.Do fails with connection refused
	errServer.Close()
	if err := statusSender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "test",
	}); err == nil {
		t.Fatal("expected transport error when SMS server is closed")
	}
}
