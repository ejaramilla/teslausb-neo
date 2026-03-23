// Package tesla provides vehicle wake-keeping interfaces and implementations
// (BLE, Tessie API) to prevent the car from sleeping while archiving.
package tesla

import "context"

// WakeKeeper keeps the vehicle awake during archive operations.
type WakeKeeper interface {
	// Start begins periodic wake signals in the background.
	Start(ctx context.Context) error

	// Stop terminates the background wake loop.
	Stop(ctx context.Context) error

	// Nudge sends a single wake signal.
	Nudge(ctx context.Context) error
}

// NoopWakeKeeper is a WakeKeeper that does nothing. It is used when wake
// functionality is disabled in configuration.
type NoopWakeKeeper struct{}

func (NoopWakeKeeper) Start(_ context.Context) error { return nil }
func (NoopWakeKeeper) Stop(_ context.Context) error  { return nil }
func (NoopWakeKeeper) Nudge(_ context.Context) error  { return nil }
