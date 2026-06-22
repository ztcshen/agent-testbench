package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/store"
)

func runEnvironmentComponents(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment components command")
	}
	switch args[0] {
	case "inspect":
		return runEnvironmentComponentsInspect(ctx, args[1:])
	case "replace":
		return runEnvironmentComponentsReplace(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment components command: %s", args[0])
	}
}

func runEnvironmentComponentsInspect(ctx context.Context, args []string) error {
	options, err := parseEnvironmentIDFlags("environment components inspect", args)
	if err != nil {
		return err
	}
	return runEnvironmentComponentsInspectWithOptions(ctx, options)
}

func runEnvironmentComponentsInspectWithOptions(ctx context.Context, options environmentIDFlags) error {
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.StoreRef, options.StoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := runtime.GetEnvironment(ctx, options.ID); err != nil {
		return err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, options.ID)
	if err != nil {
		return err
	}
	return printEnvironmentComponentGraph(options.ID, graph, options.JSONOutput)
}

func runEnvironmentComponentsReplace(ctx context.Context, args []string) error {
	options, err := parseEnvironmentComponentsReplaceOptions(args)
	if err != nil {
		return err
	}
	return runEnvironmentComponentsReplaceWithOptions(ctx, options)
}

type environmentComponentsReplaceOptions struct {
	storeRef   string
	storeURL   string
	jsonOutput bool
	id         string
	file       string
}

func parseEnvironmentComponentsReplaceOptions(args []string) (environmentComponentsReplaceOptions, error) {
	flags := flag.NewFlagSet("environment components replace", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	file := flags.String("file", "", "Component graph JSON file")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentComponentsReplaceOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentComponentsReplaceOptions{}, errors.New("environment id is required")
	}
	if strings.TrimSpace(*file) == "" {
		return environmentComponentsReplaceOptions{}, errors.New("--file COMPONENT_GRAPH_JSON is required")
	}
	return environmentComponentsReplaceOptions{
		storeRef:   *storeRef,
		storeURL:   *storeURL,
		jsonOutput: *jsonOutput,
		id:         id,
		file:       strings.TrimSpace(*file),
	}, nil
}

func runEnvironmentComponentsReplaceWithOptions(ctx context.Context, options environmentComponentsReplaceOptions) error {
	raw, err := os.ReadFile(options.file)
	if err != nil {
		return err
	}
	var graph store.EnvironmentComponentGraph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return fmt.Errorf("decode component graph JSON: %w", err)
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.storeRef, options.storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := runtime.GetEnvironment(ctx, options.id); err != nil {
		return err
	}
	readiness := environmentRestoreComponentGraphReport(options.id, graph)
	if readiness.Configured && !readiness.OK {
		return fmt.Errorf("component graph restore readiness failed: %s", readiness.Error)
	}
	if err := runtime.ReplaceEnvironmentComponentGraph(ctx, options.id, graph); err != nil {
		return err
	}
	graph, err = runtime.GetEnvironmentComponentGraph(ctx, options.id)
	if err != nil {
		return err
	}
	return printEnvironmentComponentGraph(options.id, graph, options.jsonOutput)
}

func printEnvironmentComponentGraph(envID string, graph store.EnvironmentComponentGraph, jsonOutput bool) error {
	readiness := environmentRestoreComponentGraphReport(envID, graph)
	payload := map[string]any{
		"ok":            true,
		"environmentId": envID,
		"componentGraph": map[string]any{
			"components":       graph.Components,
			"dependencies":     graph.Dependencies,
			"assets":           graph.Assets,
			"restoreReadiness": readiness,
			"counts": map[string]int{
				"components":   len(graph.Components),
				"dependencies": len(graph.Dependencies),
				"assets":       len(graph.Assets),
			},
		},
	}
	if jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Environment Component Graph: %s\n", envID)
	fmt.Printf("Components: %d\n", len(graph.Components))
	fmt.Printf("Dependencies: %d\n", len(graph.Dependencies))
	fmt.Printf("Assets: %d\n", len(graph.Assets))
	fmt.Printf("Restore-ready: %t\n", readiness.OK)
	if len(readiness.BlockingOrder) > 0 {
		fmt.Printf("Blocking order: %s\n", strings.Join(readiness.BlockingOrder, " -> "))
	}
	if strings.TrimSpace(readiness.Error) != "" {
		fmt.Printf("Readiness error: %s\n", readiness.Error)
	}
	for _, component := range graph.Components {
		label := strings.TrimSpace(component.DisplayName)
		if label == "" {
			label = component.ComponentID
		}
		fmt.Printf("- %s [%s/%s] compose=%s required=%t\n", component.ComponentID, component.Kind, component.Role, component.ComposeService, component.Required)
		if label != component.ComponentID {
			fmt.Printf("  name: %s\n", label)
		}
	}
	return nil
}
