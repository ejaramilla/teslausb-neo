package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// AppriseNotifier sends notifications via an Apprise REST API server.
type AppriseNotifier struct {
	URL string // Base URL of the Apprise API, e.g. "http://localhost:8000".
}

// NewApprise creates an AppriseNotifier pointing at the given Apprise API URL.
func NewApprise(url string) *AppriseNotifier {
	return &AppriseNotifier{URL: url}
}

// apprisePayload is the JSON body expected by the Apprise /notify endpoint.
type apprisePayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Type  string `json:"type"` // "info", "success", "warning", "failure"
}

// mapEventType converts our event types to Apprise notification types.
func mapEventType(eventType string) string {
	switch eventType {
	case EventStart:
		return "info"
	case EventFinish:
		return "success"
	case EventWarning:
		return "warning"
	case EventError:
		return "failure"
	default:
		return "info"
	}
}

// Send posts a notification to the Apprise REST API.
func (a *AppriseNotifier) Send(ctx context.Context, title, message, eventType string) error {
	payload := apprisePayload{
		Title: title,
		Body:  message,
		Type:  mapEventType(eventType),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("apprise: marshal: %w", err)
	}

	endpoint := a.URL + "/notify"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("apprise: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("apprise: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("apprise: unexpected status %d", resp.StatusCode)
	}

	return nil
}
