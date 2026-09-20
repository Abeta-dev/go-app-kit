package notifications

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBrevoSender_Validation(t *testing.T) {
	// Missing API key
	sender, err := NewBrevoSender(BrevoConfig{
		SenderEmail: "test@example.com",
	})
	assert.ErrorIs(t, err, ErrBrevoAPIKeyMissing)
	assert.Nil(t, sender)

	// Missing Sender email
	sender, err = NewBrevoSender(BrevoConfig{
		APIKey: "xkeysib-mock-key",
	})
	assert.ErrorIs(t, err, ErrBrevoSenderMissing)
	assert.Nil(t, sender)

	// Valid config
	sender, err = NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-mock-key",
		SenderEmail: "test@example.com",
		SenderName:  "Test App",
	})
	require.NoError(t, err)
	assert.NotNil(t, sender)
	assert.Equal(t, ChannelEmail, sender.Channel())
}

func TestBrevoSender_Send_Success(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody brevoSendEmailRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/smtp/email", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "xkeysib-test-12345", r.Header.Get("api-key"))

		receivedHeaders = r.Header.Clone()
		err := json.NewDecoder(r.Body).Decode(&receivedBody)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<20260915.mock@brevo.com>"}`))
	}))
	defer ts.Close()

	sender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-test-12345",
		SenderEmail: "noreply@edupro.in",
		SenderName:  "EduPro Platform",
		BaseURL:     ts.URL,
		HTTPClient:  ts.Client(),
	})
	require.NoError(t, err)

	msg := Message{
		ID:         "msg-001",
		Title:      "Welcome to EduPro",
		Body:       "Your account is ready.",
		HTMLBody:   "<h3>Welcome</h3><p>Your account is ready.</p>",
		Recipients: []string{"student@example.com", "parent@example.com"},
		Channels:   []Channel{ChannelEmail},
	}

	err = sender.Send(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, "xkeysib-test-12345", receivedHeaders.Get("api-key"))
	assert.Equal(t, "EduPro Platform", receivedBody.Sender.Name)
	assert.Equal(t, "noreply@edupro.in", receivedBody.Sender.Email)
	assert.Equal(t, "Welcome to EduPro", receivedBody.Subject)
	assert.Equal(t, "<h3>Welcome</h3><p>Your account is ready.</p>", receivedBody.HTMLContent)
	assert.Equal(t, "Your account is ready.", receivedBody.TextContent)
	require.Len(t, receivedBody.To, 2)
	assert.Equal(t, "student@example.com", receivedBody.To[0].Email)
	assert.Equal(t, "parent@example.com", receivedBody.To[1].Email)
}

func TestBrevoSender_Send_WithAttachments(t *testing.T) {
	var receivedBody brevoSendEmailRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<mock@brevo.com>"}`))
	}))
	defer ts.Close()

	sender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-test-key",
		SenderEmail: "admin@edupro.in",
		BaseURL:     ts.URL,
		HTTPClient:  ts.Client(),
	})
	require.NoError(t, err)

	fileData := []byte("Invoice-PDF-Bytes-12345")
	msg := Message{
		Title:      "Fee Invoice",
		Body:       "Please find attached receipt.",
		Recipients: []string{"parent@example.com"},
		Attachments: []Attachment{
			{
				Filename:    "receipt.pdf",
				ContentType: "application/pdf",
				Data:        fileData,
			},
		},
	}

	err = sender.Send(context.Background(), msg)
	require.NoError(t, err)

	require.Len(t, receivedBody.Attachment, 1)
	assert.Equal(t, "receipt.pdf", receivedBody.Attachment[0].Name)
	assert.Equal(t, base64.StdEncoding.EncodeToString(fileData), receivedBody.Attachment[0].Content)
}

func TestBrevoSender_Send_ErrorResponses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_parameter","message":"invalid email format"}`))
	}))
	defer ts.Close()

	sender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-mock-key",
		SenderEmail: "sender@example.com",
		BaseURL:     ts.URL,
		HTTPClient:  ts.Client(),
	})
	require.NoError(t, err)

	msg := Message{
		Title:      "Test",
		Body:       "Test",
		Recipients: []string{"invalid-email"},
	}

	err = sender.Send(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "brevo api error (status 400)")
	assert.Contains(t, err.Error(), "invalid_parameter")
}

func TestBrevoSender_Send_ContextCancellation(t *testing.T) {
	sender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-mock",
		SenderEmail: "sender@example.com",
		BaseURL:     "http://127.0.0.1:9999",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	msg := Message{
		Title:      "Hello",
		Body:       "World",
		Recipients: []string{"test@example.com"},
	}

	err = sender.Send(ctx, msg)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBrevoSender_Send_EmptyRecipients(t *testing.T) {
	sender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-mock",
		SenderEmail: "sender@example.com",
	})
	require.NoError(t, err)

	err = sender.Send(context.Background(), Message{Title: "No recipients", Body: "Empty"})
	assert.ErrorIs(t, err, ErrEmptyRecipients)
}

func TestBrevoSender_BrokerIntegration(t *testing.T) {
	dispatched := make(chan string, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req brevoSendEmailRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		dispatched <- req.Subject
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<mock@brevo.com>"}`))
	}))
	defer ts.Close()

	brevoSender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-key",
		SenderEmail: "info@edupro.in",
		BaseURL:     ts.URL,
		HTTPClient:  ts.Client(),
	})
	require.NoError(t, err)

	broker := NewBroker(Config{Workers: 2, QueueSize: 10})
	defer broker.Close()

	broker.RegisterSender(brevoSender)

	msg := Message{
		ID:         "brk-msg-1",
		Title:      "Broker Dispatched via Brevo",
		Body:       "Test body",
		Recipients: []string{"user@example.com"},
		Channels:   []Channel{ChannelEmail},
	}

	err = broker.Send(context.Background(), msg)
	require.NoError(t, err)

	select {
	case subject := <-dispatched:
		assert.Equal(t, "Broker Dispatched via Brevo", subject)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for brevo message delivery")
	}
}

func TestBrevoSender_EdgeCases(t *testing.T) {
	sender, err := NewBrevoSender(BrevoConfig{
		APIKey:      "xkeysib-mock",
		SenderEmail: "sender@example.com",
	})
	require.NoError(t, err)

	// 1. Send with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = sender.Send(ctx, Message{Recipients: []string{"test@example.com"}})
	assert.ErrorIs(t, err, context.Canceled)

	// 2. buildBrevoPayload with blank-only recipients
	_, err = buildBrevoPayload(Message{Recipients: []string{"   ", "\t"}}, "Sender", "sender@example.com")
	assert.ErrorIs(t, err, ErrEmptyRecipients)

	// 3. buildBrevoPayload with text body only (generates HTML fallback)
	payload, err := buildBrevoPayload(Message{
		Recipients: []string{"a@b.com"},
		Body:       "Line 1\nLine 2",
	}, "Sender", "sender@example.com")
	require.NoError(t, err)
	assert.Equal(t, "<div>Line 1<br/>Line 2</div>", payload.HTMLContent)
	assert.Equal(t, "Line 1\nLine 2", payload.TextContent)

	// 4. buildBrevoPayload with HTML body only (generates text fallback)
	payload2, err := buildBrevoPayload(Message{
		Recipients: []string{"a@b.com"},
		HTMLBody:   "<p>Hello</p>",
	}, "Sender", "sender@example.com")
	require.NoError(t, err)
	assert.Equal(t, "<p>Hello</p>", payload2.HTMLContent)
	assert.Equal(t, "<p>Hello</p>", payload2.TextContent)
}
