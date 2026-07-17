package main

import (
	"context"
	"fmt"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runTrustedTestKitBatch(ctx context.Context, bundle profile.Bundle, runtime store.Store, cases []profile.APICase, baseURL string, timeoutSeconds int, failureLabel string) (map[string]any, error) {
	started := time.Now()
	results := make([]map[string]any, 0, len(cases))
	passed := 0
	for _, item := range cases {
		result, err := controlplane.RunTrustedTestKitCase(ctx, bundle, runtime, controlplane.TrustedTestKitRunRequest{
			CaseID:         item.ID,
			BaseURL:        baseURL,
			TimeoutSeconds: timeoutSeconds,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: execute case %s: %w", failureLabel, item.ID, err)
		}
		delete(result, "httpStatus")
		if result["ok"] == true {
			passed++
		}
		results = append(results, result)
	}
	return map[string]any{
		"ok":        passed == len(results),
		"results":   results,
		"elapsedMs": time.Since(started).Milliseconds(),
		"summary": map[string]any{
			"caseCount": len(results),
			"passed":    passed,
			"failed":    len(results) - passed,
		},
	}, nil
}
