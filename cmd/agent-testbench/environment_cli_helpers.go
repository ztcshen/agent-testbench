package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/environmentprojection"
	"agent-testbench/internal/store"
)

type environmentIDFlags struct {
	StoreRef   string
	StoreURL   string
	ID         string
	JSONOutput bool
}

func parseEnvironmentIDFlags(name string, args []string) (environmentIDFlags, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentIDFlags{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentIDFlags{}, errors.New("environment id is required")
	}
	return environmentIDFlags{
		StoreRef:   *storeRef,
		StoreURL:   *storeURL,
		ID:         id,
		JSONOutput: *jsonOutput,
	}, nil
}

func openRequiredCLIStore(ctx context.Context, storeRef string, legacyStoreURL string) (store.Store, func(), error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, legacyStoreURL)
	if err != nil {
		return nil, func() {}, err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return nil, func() {}, err
	}
	return runtime, cleanupCLIStore(runtime), nil
}

func cleanupCLIStore(runtime store.Store) func() {
	return func() {
		closeCLIStore(runtime)
	}
}

func closeCLIStore(runtime store.Store) {
	if runtime == nil {
		return
	}
	if err := runtime.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: close store: %v\n", err)
	}
}

func loadEnvironmentAndComponentGraphForCLI(ctx context.Context, storeRef string, legacyStoreURL string, id string) (store.Environment, store.EnvironmentComponentGraph, error) {
	runtime, cleanup, err := openRequiredCLIStore(ctx, storeRef, legacyStoreURL)
	if err != nil {
		return store.Environment{}, store.EnvironmentComponentGraph{}, err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return store.Environment{}, store.EnvironmentComponentGraph{}, err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, id)
	if err != nil {
		return store.Environment{}, store.EnvironmentComponentGraph{}, err
	}
	return env, graph, nil
}

func printEnvironmentCommandResult(env store.Environment, jsonOutput bool, componentGraphs ...store.EnvironmentComponentGraph) error {
	payload := map[string]any{"ok": true, "environment": environmentPayload(env)}
	if len(componentGraphs) > 0 {
		payload["componentGraph"] = environmentRestoreComponentGraphReport(env.ID, componentGraphs[0])
		payload["fileProjection"] = environmentprojection.FromEnvironment(env, componentGraphs[0])
	}
	if jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Environment: %s\n", env.ID)
	fmt.Printf("Status: %s\n", env.Status)
	fmt.Printf("Verified: %t\n", env.Verified)
	if env.VerificationWorkflowID != "" {
		fmt.Printf("Verification Workflow: %s\n", env.VerificationWorkflowID)
	}
	if env.LastVerificationRunID != "" {
		fmt.Printf("Last Verification Run: %s [%s]\n", env.LastVerificationRunID, env.LastVerificationStatus)
	}
	fmt.Printf("Evidence Complete: %t\n", env.EvidenceComplete)
	fmt.Printf("SkyWalking Topology Complete: %t\n", env.TopologyComplete)
	if len(componentGraphs) > 0 {
		readiness := environmentRestoreComponentGraphReport(env.ID, componentGraphs[0])
		fmt.Printf("Component Restore-ready: %t\n", readiness.OK)
		if len(readiness.BlockingOrder) > 0 {
			fmt.Printf("Component Blocking Order: %s\n", strings.Join(readiness.BlockingOrder, " -> "))
		}
		if strings.TrimSpace(readiness.Error) != "" {
			fmt.Printf("Component Readiness Error: %s\n", readiness.Error)
		}
		projection := environmentprojection.FromEnvironment(env, componentGraphs[0])
		fmt.Printf("File Projection Ready: %t\n", projection.OK)
		if len(projection.Missing) > 0 {
			fmt.Printf("File Projection Missing: %d\n", len(projection.Missing))
		}
	}
	return nil
}

func environmentPayloads(items []store.Environment) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, environmentPayload(item))
	}
	return out
}

func environmentPayload(env store.Environment) map[string]any {
	payload := map[string]any{
		"id":                     env.ID,
		"displayName":            env.DisplayName,
		"description":            env.Description,
		"status":                 env.Status,
		"verified":               env.Verified,
		"services":               jsonArrayString(env.ServicesJSON),
		"repos":                  jsonObjectString(env.ReposJSON),
		"compose":                jsonObjectString(env.ComposeJSON),
		"healthChecks":           jsonArrayString(env.HealthChecksJSON),
		"verificationWorkflowId": env.VerificationWorkflowID,
		"lastVerificationRunId":  env.LastVerificationRunID,
		"lastVerificationStatus": env.LastVerificationStatus,
		"evidenceComplete":       env.EvidenceComplete,
		"topologyComplete":       env.TopologyComplete,
		"summary":                jsonObjectString(env.SummaryJSON),
		"createdAt":              env.CreatedAt,
		"updatedAt":              env.UpdatedAt,
	}
	if !env.LastVerifiedAt.IsZero() {
		payload["lastVerifiedAt"] = env.LastVerifiedAt
	}
	return payload
}
