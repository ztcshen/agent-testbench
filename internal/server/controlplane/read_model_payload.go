package controlplane

import (
	"context"
	"encoding/json"
	"errors"

	"agent-testbench/internal/store"
)

func readModelPayload[T any](ctx context.Context, runtime store.Store, profileID string, key string) (T, bool, error) {
	var payload T
	model, err := runtime.GetReadModel(ctx, profileID, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return payload, false, nil
		}
		return payload, false, err
	}
	if err := json.Unmarshal([]byte(model.PayloadJSON), &payload); err != nil {
		return payload, false, err
	}
	return payload, true, nil
}
