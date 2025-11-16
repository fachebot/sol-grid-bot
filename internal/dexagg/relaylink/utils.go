package relaylink

import (
	"context"
	"fmt"

	"github.com/fachebot/sol-grid-bot/internal/ent/settings"

	"github.com/carlmjohnson/requests"
)

type Percentiles struct {
	Sixty       uint64 `json:"60"`
	SeventyFive uint64 `json:"75"`
	EightyFive  uint64 `json:"85"`
}

type PerComputeUnit struct {
	Percentiles Percentiles `json:"percentiles"`
}

type SolanaGasTracker struct {
	PerComputeUnit PerComputeUnit `json:"per_compute_unit"`
}

type SolanaGasTrackerResponse struct {
	Sol SolanaGasTracker `json:"sol"`
}

func getPriorityFeeByLevel(ctx context.Context, level settings.PriorityLevel) (uint64, error) {
	var response SolanaGasTrackerResponse
	err := requests.URL("https://quicknode.com/_gas-tracker?slug=solana").
		ToJSON(&response).
		Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("gas tracker: %w", err)
	}

	switch level {
	case settings.PriorityLevelMedium:
		return response.Sol.PerComputeUnit.Percentiles.Sixty, nil
	case settings.PriorityLevelHigh:
		return response.Sol.PerComputeUnit.Percentiles.SeventyFive, nil
	case settings.PriorityLevelVeryHigh:
		return response.Sol.PerComputeUnit.Percentiles.EightyFive, nil
	default:
		return 0, nil
	}
}
