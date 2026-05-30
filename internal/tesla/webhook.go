package tesla

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// WebhookWakeKeeper keeps the vehicle awake by POSTing to a user-supplied URL
// at the start/stop/nudge points of an archive cycle. The body is a small JSON
// object {"action":"start|stop|nudge"} so the receiver (Home Assistant, a
// serverless function, etc.) can decide what to do. This mirrors the original
// teslausb KEEP_AWAKE_WEBHOOK_URL contract.
type WebhookWakeKeeper struct {
	URL    string
	client *http.Client
}

// NewWebhookWakeKeeper creates a WebhookWakeKeeper for the given URL.
func NewWebhookWakeKeeper(url string) *WebhookWakeKeeper {
	return &WebhookWakeKeeper{URL: url, client: http.DefaultClient}
}

// Start signals the receiver that an archive cycle is beginning.
func (k *WebhookWakeKeeper) Start(ctx context.Context) error {
	return k.post(ctx, "start")
}

// Stop signals the receiver that the archive cycle is complete (e.g. so it can
// let the car sleep again).
func (k *WebhookWakeKeeper) Stop(ctx context.Context) error {
	return k.post(ctx, "stop")
}

// Nudge sends a single keep-awake signal.
func (k *WebhookWakeKeeper) Nudge(ctx context.Context) error {
	return k.post(ctx, "nudge")
}

func (k *WebhookWakeKeeper) post(ctx context.Context, action string) error {
	body := []byte(fmt.Sprintf(`{"action":%q}`, action))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: %s request: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: %s unexpected status %d", action, resp.StatusCode)
	}
	return nil
}
