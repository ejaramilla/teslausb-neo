// Package notify provides notification dispatching to multiple backends
// (ntfy, Apprise, etc.).
package notify

import (
	"context"
	"fmt"
	"strings"
)

// Event type constants used when sending notifications.
const (
	EventStart   = "start"
	EventFinish  = "finish"
	EventWarning = "warning"
	EventError   = "error"
)

// Notifier is implemented by every notification backend.
type Notifier interface {
	// Send dispatches a notification. eventType is one of the Event*
	// constants defined in this package.
	Send(ctx context.Context, title, message, eventType string) error
}

// Multi wraps multiple Notifiers and fans out every Send call to all of them.
type Multi struct {
	notifiers []Notifier
}

// NewMulti creates a Multi dispatcher from the supplied notifiers.
// Nil entries are silently ignored.
func NewMulti(notifiers ...Notifier) *Multi {
	valid := make([]Notifier, 0, len(notifiers))
	for _, n := range notifiers {
		if n != nil {
			valid = append(valid, n)
		}
	}
	return &Multi{notifiers: valid}
}

// Send dispatches the notification to every wrapped Notifier.
// Errors are collected; a combined error is returned if any backend fails.
func (m *Multi) Send(ctx context.Context, title, message, eventType string) error {
	var errs []string
	for _, n := range m.notifiers {
		if err := n.Send(ctx, title, message, eventType); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify: %s", strings.Join(errs, "; "))
	}
	return nil
}
