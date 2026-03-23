package tesla

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// BLEWakeKeeper keeps the vehicle awake by periodically invoking the
// tesla-control CLI tool over BLE.
type BLEWakeKeeper struct {
	VIN        string
	BinaryPath string // Path to the tesla-control binary.

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewBLEWakeKeeper creates a BLEWakeKeeper.
func NewBLEWakeKeeper(vin, binaryPath string) *BLEWakeKeeper {
	if binaryPath == "" {
		binaryPath = "tesla-control"
	}
	return &BLEWakeKeeper{
		VIN:        vin,
		BinaryPath: binaryPath,
	}
}

// Start launches a background goroutine that periodically nudges the vehicle.
func (b *BLEWakeKeeper) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		return nil // already running
	}

	loopCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.done = make(chan struct{})

	go b.loop(loopCtx)
	return nil
}

// Stop signals the background goroutine to exit and waits for it.
func (b *BLEWakeKeeper) Stop(_ context.Context) error {
	b.mu.Lock()
	cancel := b.cancel
	done := b.done
	b.cancel = nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
	return nil
}

// Nudge sends a single BLE wake command via tesla-control.
func (b *BLEWakeKeeper) Nudge(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, b.BinaryPath, "-vin", b.VIN, "-ble", "body-controller-state")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ble nudge: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// loop runs the periodic nudge until the context is cancelled.
func (b *BLEWakeKeeper) loop(ctx context.Context) {
	defer close(b.done)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Nudge immediately on start.
	_ = b.Nudge(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = b.Nudge(ctx)
		}
	}
}
