// SPDX-License-Identifier: MIT

package notifications_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/umesh0492/go-app-kit/notifications"
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
		EntityID: "DLT123456",
		SenderID: "TXTSMS",
		APIKey:   "secret-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create SMS sender: %v", err)
	}

	err = sender.Send(context.Background(), notifications.Message{
		Recipients: []string{"+919876543210"},
		Body:       "Your code is 654321",
	})
	if err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}

	if !strings.Contains(receivedBody, "numbers=919876543210") || !strings.Contains(receivedBody, "pe_id=DLT123456") {
		t.Fatalf("unexpected post body: %s", receivedBody)
	}
}

func TestSMSSender_MissingConfig(t *testing.T) {
	_, err := notifications.NewSMSSender(notifications.SMSConfig{})
	if err == nil {
		t.Fatal("expected error when endpoint or key is missing and not simulated")
	}
}
