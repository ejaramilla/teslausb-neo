package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// mockNotifier records calls to Send.
type mockNotifier struct {
	mu       sync.Mutex
	calls    []mockCall
	failWith error
}

type mockCall struct {
	Title     string
	Message   string
	EventType string
}

func (m *mockNotifier) Send(_ context.Context, title, message, eventType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockCall{title, message, eventType})
	return m.failWith
}

func TestMultiNotifier(t *testing.T) {
	n1 := &mockNotifier{}
	n2 := &mockNotifier{}
	multi := NewMulti(n1, n2)

	ctx := context.Background()
	err := multi.Send(ctx, "Archive Complete", "10 files archived", EventFinish)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	for i, n := range []*mockNotifier{n1, n2} {
		n.mu.Lock()
		if len(n.calls) != 1 {
			t.Errorf("notifier %d: got %d calls, want 1", i, len(n.calls))
			n.mu.Unlock()
			continue
		}
		c := n.calls[0]
		n.mu.Unlock()

		if c.Title != "Archive Complete" {
			t.Errorf("notifier %d: title = %q, want %q", i, c.Title, "Archive Complete")
		}
		if c.Message != "10 files archived" {
			t.Errorf("notifier %d: message = %q, want %q", i, c.Message, "10 files archived")
		}
		if c.EventType != EventFinish {
			t.Errorf("notifier %d: eventType = %q, want %q", i, c.EventType, EventFinish)
		}
	}
}

func TestMultiNotifierNilIgnored(t *testing.T) {
	n1 := &mockNotifier{}
	multi := NewMulti(nil, n1, nil)

	ctx := context.Background()
	if err := multi.Send(ctx, "test", "msg", EventStart); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	n1.mu.Lock()
	defer n1.mu.Unlock()
	if len(n1.calls) != 1 {
		t.Errorf("got %d calls, want 1", len(n1.calls))
	}
}

func TestNtfySend(t *testing.T) {
	var (
		gotMethod   string
		gotTitle    string
		gotPriority string
		gotTags     string
		gotAuth     string
		gotBody     string
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotTitle = r.Header.Get("Title")
		gotPriority = r.Header.Get("Priority")
		gotTags = r.Header.Get("Tags")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	notifier := NewNtfy(ts.URL, "high", "my-token")

	ctx := context.Background()
	err := notifier.Send(ctx, "Test Title", "Test body message", EventWarning)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotTitle != "Test Title" {
		t.Errorf("Title header = %q, want %q", gotTitle, "Test Title")
	}
	if gotPriority != "high" {
		t.Errorf("Priority header = %q, want %q", gotPriority, "high")
	}
	if gotTags != EventWarning {
		t.Errorf("Tags header = %q, want %q", gotTags, EventWarning)
	}
	if gotAuth != "Bearer my-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-token")
	}
	if gotBody != "Test body message" {
		t.Errorf("body = %q, want %q", gotBody, "Test body message")
	}
}

func TestNtfySendDefaultPriority(t *testing.T) {
	notifier := NewNtfy("http://example.com", "", "")
	if notifier.Priority != "default" {
		t.Errorf("Priority = %q, want %q", notifier.Priority, "default")
	}
}

func TestNtfySendNoAuth(t *testing.T) {
	var gotAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	notifier := NewNtfy(ts.URL, "default", "")
	ctx := context.Background()
	if err := notifier.Send(ctx, "t", "m", EventStart); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty (no token)", gotAuth)
	}
}

func TestAppriseSend(t *testing.T) {
	var (
		gotMethod      string
		gotContentType string
		gotPayload     apprisePayload
		gotPath        string
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	notifier := NewApprise(ts.URL)

	ctx := context.Background()
	err := notifier.Send(ctx, "Archive Error", "disk full", EventError)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
	if gotPath != "/notify" {
		t.Errorf("path = %q, want %q", gotPath, "/notify")
	}
	if gotPayload.Title != "Archive Error" {
		t.Errorf("payload title = %q, want %q", gotPayload.Title, "Archive Error")
	}
	if gotPayload.Body != "disk full" {
		t.Errorf("payload body = %q, want %q", gotPayload.Body, "disk full")
	}
	if gotPayload.Type != "failure" {
		t.Errorf("payload type = %q, want %q", gotPayload.Type, "failure")
	}
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{EventStart, "info"},
		{EventFinish, "success"},
		{EventWarning, "warning"},
		{EventError, "failure"},
		{"unknown", "info"},
		{"", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapEventType(tt.input)
			if got != tt.want {
				t.Errorf("mapEventType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
