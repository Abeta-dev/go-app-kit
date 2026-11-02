---
name: go-app-kit-notifications
description: "Dispatch synchronous and asynchronous multi-channel notifications (SMTP Email, Meta WhatsApp Cloud API, Indian DLT SMS, Slack, and HMAC-SHA256 Webhooks) using github.com/umesh0492/go-app-kit/notifications. Use when wiring notification brokers, sending email attachments, or verifying webhook signatures."
---

# `go-app-kit/notifications` — Multi-Channel Notification Broker Subskill

Use this skill when dispatching notifications across Email, WhatsApp, SMS, Slack, or signed Webhooks via `github.com/umesh0492/go-app-kit/notifications`.

## 1. Broker Setup & Pluggable Channel Senders

`notifications.Broker` uses `github.com/umesh0492/go-libs/workerpool` under the hood for bounded, panic-safe asynchronous fan-out:

```go
import (
	"context"
	"github.com/umesh0492/go-app-kit/notifications"
)

cfg := notifications.DefaultConfig()
cfg.Workers = 8
cfg.QueueSize = 256

broker := notifications.NewBroker(cfg)
defer broker.Close() // drains in-flight async tasks gracefully

// Register only the channels your service uses:
broker.RegisterSender(notifications.NewEmailSender(emailCfg))       // RFC 2047 MIME + attachments
broker.RegisterSender(notifications.NewWhatsAppSender(whatsappCfg)) // Meta Cloud API v20+
broker.RegisterSender(notifications.NewSMSSender(smsCfg))           // Indian DLT (pe_id, template_id)
broker.RegisterSender(notifications.NewSlackSender(slackCfg))       // Priority color-coded blocks
broker.RegisterSender(notifications.NewWebhookSender(webhookCfg))   // HMAC-SHA256 signed POST
```

---

## 2. Synchronous vs. Asynchronous Dispatch

Use `broker.Send(ctx, msg)` when the caller must block on delivery confirmation, or `broker.SendAsync(ctx, msg)` for non-blocking background fan-out:

```go
err := broker.SendAsync(ctx, notifications.Message{
	ID:         "msg-inv-100",
	Title:      "Tax Invoice INV-100 Issued",
	Body:       "Please find your GST tax invoice attached.",
	Priority:   notifications.PriorityHigh,
	Recipients: []string{"accounts@buyer.in"},
	Channels:   []notifications.Channel{notifications.ChannelEmail, notifications.ChannelSlack},
	Attachments: []notifications.Attachment{
		{
			Filename:    "INV-100.pdf",
			ContentType: "application/pdf",
			Data:        pdfBytes,
		},
	},
})
```

---

## 3. Outbound & Inbound Webhook Security (HMAC-SHA256 + Replay Defense)

`NewWebhookSender` signs payloads with `X-Signature-SHA256` and binds the timestamp. On the receiving end, always verify using `WebhookVerifier` (enforces constant-time HMAC comparison and a mandatory 5-minute replay window):

```go
verifier := notifications.NewWebhookVerifier(sharedSecret)

if err := verifier.Verify(payloadBytes, signatureHeader, timestampHeader); err != nil {
	// Reject with HTTP 401 Unauthorized
}
```
