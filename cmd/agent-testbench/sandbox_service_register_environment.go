package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func hydrateSandboxServiceRegistrationFromEnvironment(ctx context.Context, runtime store.Store, environmentID string, request *controlplane.SandboxServiceRegistrationRequest) error {
	if strings.TrimSpace(environmentID) == "" {
		return nil
	}
	serviceID := strings.TrimSpace(request.ID)
	if serviceID == "" {
		return errors.New("--id is required with --from-environment")
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, environmentID)
	if err != nil {
		return fmt.Errorf("load environment component graph %s: %w", environmentID, err)
	}
	component, ok := sandboxComponentForServiceID(graph.Components, serviceID)
	if !ok {
		return fmt.Errorf("environment %s has no component matching service %s", environmentID, serviceID)
	}
	if strings.TrimSpace(request.DisplayName) == "" {
		request.DisplayName = component.DisplayName
	}
	if strings.TrimSpace(request.Kind) == "" {
		request.Kind = component.Kind
	}
	if strings.TrimSpace(request.StartupCommand) == "" {
		request.StartupCommand = sandboxComponentStartupCommand(component)
	}
	if strings.TrimSpace(request.StartupCommand) == "" {
		return fmt.Errorf("environment %s component %s has no startupCommand/startCommand metadata", environmentID, component.ComponentID)
	}
	return nil
}

func sandboxComponentForServiceID(components []store.EnvironmentComponent, serviceID string) (store.EnvironmentComponent, bool) {
	for _, component := range components {
		if strings.TrimSpace(component.ComponentID) == serviceID || strings.TrimSpace(component.ComposeService) == serviceID {
			return component, true
		}
	}
	return store.EnvironmentComponent{}, false
}

func sandboxComponentStartupCommand(component store.EnvironmentComponent) string {
	for _, raw := range []string{component.RuntimeJSON, component.SummaryJSON} {
		values := jsonObjectString(raw)
		for _, key := range []string{"startupCommand", "startCommand"} {
			if value := strings.TrimSpace(valueString(values[key])); value != "" {
				return value
			}
		}
		sandbox := mapFromReportAny(values["sandbox"])
		for _, key := range []string{"startupCommand", "startCommand"} {
			if value := strings.TrimSpace(valueString(sandbox[key])); value != "" {
				return value
			}
		}
	}
	return ""
}
