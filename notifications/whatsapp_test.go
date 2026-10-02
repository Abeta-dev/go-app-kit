// SPDX-License-Identifier: MIT

package notifications_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/umesh0492/go-app-kit/notifications"
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
		Recipients: []string{"+919876543210"},
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

func TestWhatsAppSender_MissingCredentials(t *testing.T) {
	_, err := notifications.NewWhatsAppSender(notifications.WhatsAppConfig{})
	if err == nil {
		t.Fatal("expected error when credentials missing and not simulated")
	}
}
