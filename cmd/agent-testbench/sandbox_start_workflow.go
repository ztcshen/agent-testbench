package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

const sandboxStartupCommandEmpty = "startup command is empty"

func sandboxWorkflowRequiredServiceReasons(catalog store.ProfileCatalog, workflowID string) (map[string]string, error) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, nil
	}
	nodes := make(map[string]store.CatalogInterfaceNode, len(catalog.InterfaceNodes))
	for _, node := range catalog.InterfaceNodes {
		nodes[node.ID] = node
	}
	services := make(map[string]bool, len(catalog.Services))
	for _, service := range catalog.Services {
		services[service.ID] = true
	}
	reasons := map[string]string{}
	bindingCount := 0
	for _, binding := range catalog.WorkflowBindings {
		if binding.WorkflowID != workflowID {
			continue
		}
		if !binding.Required {
			continue
		}
		bindingCount++
		nodeID := strings.TrimSpace(binding.NodeID)
		node, ok := nodes[nodeID]
		if !ok {
			return nil, fmt.Errorf("workflow %s binding %s references missing interface node %s", workflowID, strings.TrimSpace(binding.StepID), nodeID)
		}
		serviceID := strings.TrimSpace(node.ServiceID)
		if serviceID == "" {
			return nil, fmt.Errorf("workflow %s binding %s interface node %s has no service id", workflowID, strings.TrimSpace(binding.StepID), nodeID)
		}
		if !services[serviceID] {
			return nil, fmt.Errorf("workflow %s binding %s references service %s that is not registered in the profile service registry", workflowID, strings.TrimSpace(binding.StepID), serviceID)
		}
		reasons[serviceID] = strings.TrimSpace("required by workflow " + workflowID + " step " + strings.TrimSpace(binding.StepID))
	}
	if bindingCount == 0 {
		return nil, fmt.Errorf("workflow has no required service bindings: %s", workflowID)
	}
	return reasons, nil
}

func sandboxRequiredStartupReason(serviceID string, serviceFilter string, workflowReason string) string {
	if strings.TrimSpace(workflowReason) != "" {
		return strings.TrimSpace(workflowReason)
	}
	if strings.TrimSpace(serviceFilter) != "" && strings.TrimSpace(serviceID) == strings.TrimSpace(serviceFilter) {
		return "explicitly requested service"
	}
	return ""
}

func runSandboxServiceStartup(ctx context.Context, service store.CatalogService, timeout time.Duration, dryRun bool, requiredReason string) sandboxStartServiceResult {
	command := strings.TrimSpace(service.StartupCommand)
	result := sandboxStartServiceResult{
		ID:             service.ID,
		DisplayName:    service.DisplayName,
		Kind:           service.Kind,
		ContainerName:  service.ContainerName,
		ServicePort:    service.ServicePort,
		ManagementPort: service.ManagementPort,
		Command:        command,
		ExitCode:       0,
	}
	if strings.TrimSpace(service.Status) != "" && strings.TrimSpace(service.Status) != "active" {
		result.Skipped = true
		result.SkipReason = "service is not active"
		return result
	}
	if command == "" {
		if strings.TrimSpace(requiredReason) != "" {
			result.ExitCode = 1
			result.Error = fmt.Sprintf("%s (%s); repair with: agent-testbench sandbox service register --id %s --startup-command '...'", sandboxStartupCommandEmpty, requiredReason, service.ID)
			return result
		}
		result.Skipped = true
		result.SkipReason = sandboxStartupCommandEmpty
		return result
	}
	if dryRun {
		result.Planned = true
		return result
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "/bin/sh", "-c", command)
	output, err := cmd.CombinedOutput()
	result.Output = strings.TrimSpace(string(output))
	if commandCtx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Error = "startup command timed out"
		return result
	}
	if err != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Error = err.Error()
	}
	return result
}
