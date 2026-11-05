// SPDX-License-Identifier: MIT

package notifications_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Abeta-dev/go-app-kit/notifications"
)

func TestWhatsAppSender_Simulated(t *testing.T) {
	sender, err := notifications.NewWhatsAppSender(notifications.WhatsAppConfig{
		IsSimulated: true,
	})
	if err != nil {
		t.Fatalf("expected nil error for simulated sender, got: %v", err)
	}

	if sender.Channel() != notifications.ChannelWhatsApp {
		t.Fatalf("expected ChannelWhatsApp, got: %s", sender.Channel())
	}

	err = sender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "Test notification",
	})
	if err != nil {
		t.Fatalf("expected nil error for simulated send, got: %v", err)
	}
}

func TestWhatsAppSender_LiveHTTP(t *testing.T) {
	var receivedAuth string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.12345"}]}`))
	}))
	defer server.Close()

	sender, err := notifications.NewWhatsAppSender(notifications.WhatsAppConfig{
		AccessToken:   "test-access-token",
		PhoneNumberID: "100200300",
		BaseURL:       server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create whatsapp sender: %v", err)
	}

	err = sender.Send(context.Background(), notifications.Message{
		Recipients: []string{"   ", "+", "+919876543210"},
		Body:       "Hello via WhatsApp",
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}

	if receivedAuth != "Bearer test-access-token" {
		t.Fatalf("unexpected Authorization header: %s", receivedAuth)
	}
	if receivedBody == "" {
		t.Fatal("expected non-empty request body sent to server")
	}
}

func TestWhatsAppSender_MissingCredentialsAndErrors(t *testing.T) {
	_, err := notifications.NewWhatsAppSender(notifications.WhatsAppConfig{})
	if !errors.Is(err, notifications.ErrWhatsAppTokenMissing) {
		t.Fatalf("expected ErrWhatsAppTokenMissing, got %v", err)
	}

	_, err = notifications.NewWhatsAppSender(notifications.WhatsAppConfig{
		AccessToken: "token-only",
	})
	if !errors.Is(err, notifications.ErrWhatsAppPhoneIDMissing) {
		t.Fatalf("expected ErrWhatsAppPhoneIDMissing, got %v", err)
	}

	// 1. Invalid URL error in NewRequestWithContext
	badURLSender, err := notifications.NewWhatsAppSender(notifications.WhatsAppConfig{
		AccessToken:   "tok",
		PhoneNumberID: "123",
		BaseURL:       "http://\x7f",
	})
	if err != nil {
		t.Fatalf("NewWhatsAppSender failed: %v", err)
	}
	if err := badURLSender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "test",
	}); err == nil {
		t.Fatal("expected error for invalid WhatsApp BaseURL")
	}

	// 2. Non-2xx status and transport error
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	statusSender, _ := notifications.NewWhatsAppSender(notifications.WhatsAppConfig{
		AccessToken:   "tok",
		PhoneNumberID: "123",
		BaseURL:       errServer.URL,
	})
	if err := statusSender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "test",
	}); err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status 400 error, got: %v", err)
	}

	errServer.Close()
	if err := statusSender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "test",
	}); err == nil {
		t.Fatal("expected transport error when WhatsApp server is closed")
	}
}
