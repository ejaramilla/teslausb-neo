package notify

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// NtfyNotifier sends notifications via the ntfy.sh HTTP API.
type NtfyNotifier struct {
	URL      string // Full topic URL, e.g. "https://ntfy.sh/my-topic".
	Priority string // ntfy priority: "min", "low", "default", "high", "max".
	Token    string // Optional access token for authenticated topics.
}

// NewNtfy creates an NtfyNotifier.
func NewNtfy(url, priority, token string) *NtfyNotifier {
	if priority == "" {
		priority = "default"
	}
	return &NtfyNotifier{
		URL:      url,
		Priority: priority,
		Token:    token,
	}
}

// Send posts a notification to the configured ntfy topic.
func (n *NtfyNotifier) Send(ctx context.Context, title, message, eventType string) error {
	body := bytes.NewBufferString(message)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, body)
	if err != nil {
		return fmt.Errorf("ntfy: build request: %w", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", n.Priority)
	req.Header.Set("Tags", eventType)

	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: unexpected status %d", resp.StatusCode)
	}

	return nil
}
