package main

import (
	"context"
	"fmt"

	"agent-testbench/internal/store"
)

const (
	mapCaseRunnerHTTP            = "http"
	mapCaseRunnerHTTPS           = "https"
	mapCaseRunnerOpenAPI         = "openapi"
	mapCaseRunnerKarate          = "karate"
	mapCaseRunnerExecutorHTTP    = "executor.http"
	mapCaseRunnerExecutorOpenAPI = "executor.openapi"
	mapCaseRunnerExecutorKarate  = "executor.karate"
)

type mapCaseRunRequest struct {
	Instance  store.TestMapPlanInstance
	Task      store.TestMapPlanTask
	Step      store.TestPlanPathStep
	CaseID    string
	RunID     string
	Overrides map[string]any
}

type mapCaseRunner interface {
	Run(context.Context, mapCaseRunRequest) (map[string]any, error)
}

type mapHTTPCaseRunner struct {
	executor mapRunExecutor
}

func (r mapHTTPCaseRunner) Run(ctx context.Context, request mapCaseRunRequest) (map[string]any, error) {
	payload := r.executor.catalogCasePayload(request)
	return runCatalogCaseOnRuntime(ctx, r.executor.runtime, request.Instance.ProfileID, payload)
}

type unsupportedMapCaseRunner struct {
	runnerID   string
	sourceKind string
}

func (r unsupportedMapCaseRunner) Run(context.Context, mapCaseRunRequest) (map[string]any, error) {
	err := fmt.Errorf("unsupported map case runner %q for sourceKind=%q", r.runnerID, r.sourceKind)
	return map[string]any{"status": store.StatusFailed, "error": err.Error()}, err
}
