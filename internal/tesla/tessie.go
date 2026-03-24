package tesla

import (
	"context"
	"fmt"
	"net/http"
)

const tessieBaseURL = "https://api.tessie.com"

// TessieWakeKeeper wakes the vehicle via the Tessie cloud API.
type TessieWakeKeeper struct {
	APIToken string
	VIN      string
}

// NewTessieWakeKeeper creates a TessieWakeKeeper.
func NewTessieWakeKeeper(apiToken, vin string) *TessieWakeKeeper {
	return &TessieWakeKeeper{
		APIToken: apiToken,
		VIN:      vin,
	}
}

// Start is a no-op; Tessie does not require a background loop.
func (t *TessieWakeKeeper) Start(_ context.Context) error { return nil }

// Stop is a no-op.
func (t *TessieWakeKeeper) Stop(_ context.Context) error { return nil }

// Nudge sends a single wake request to the Tessie API.
func (t *TessieWakeKeeper) Nudge(ctx context.Context) error {
	url := fmt.Sprintf("%s/%s/wake", tessieBaseURL, t.VIN)
	return t.nudgeWithURL(ctx, url)
}

// nudgeWithURL sends a wake POST to the given URL. Extracted for testability.
func (t *TessieWakeKeeper) nudgeWithURL(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("tessie: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.APIToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tessie: wake request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tessie: unexpected status %d", resp.StatusCode)
	}

	return nil
}
