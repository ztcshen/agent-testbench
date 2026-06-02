package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"

	"agent-testbench/internal/domain/profile"
)

type workflowListReport struct {
	OK        bool               `json:"ok"`
	ProfileID string             `json:"profileId"`
	ServiceID string             `json:"serviceId,omitempty"`
	Count     int                `json:"count"`
	Items     []workflowListItem `json:"items"`
}

type workflowListItem struct {
	ID           string                     `json:"id"`
	DisplayName  string                     `json:"displayName,omitempty"`
	Description  string                     `json:"description,omitempty"`
	StepCount    int                        `json:"stepCount"`
	ServiceIDs   []string                   `json:"serviceIds,omitempty"`
	MatchedSteps []workflowServiceMatchStep `json:"matchedSteps,omitempty"`
}

type workflowServiceMatchStep struct {
	StepID    string `json:"stepId"`
	NodeID    string `json:"nodeId"`
	CaseID    string `json:"caseId,omitempty"`
	ServiceID string `json:"serviceId"`
	Required  bool   `json:"required,omitempty"`
	SortOrder int    `json:"sortOrder,omitempty"`
}

func runWorkflowDiscover(ctx context.Context, args []string) error {
	options, err := parseWorkflowDiscoverCommandOptions(args)
	if err != nil {
		return err
	}
	bundle, cleanup, err := options.loadDiscoveryBundle(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	report := workflowList(bundle, options.Filter, options.ServiceID)
	if options.JSONOutput {
		return writeIndentedJSON(report)
	}
	for _, item := range report.Items {
		fmt.Printf("%s\t%s\t%d\n", item.ID, item.DisplayName, item.StepCount)
	}
	return nil
}

func parseWorkflowDiscoverCommandOptions(args []string) (profileDiscoveryCommandOptions, error) {
	return parseProfileDiscoveryCommandOptionsWith("workflow discover", "Filter by id, display name, or description", args, func(flags *flag.FlagSet) func(*profileDiscoveryCommandOptions) {
		serviceID := flags.String("service", "", "Filter workflows by service id used in workflow bindings")
		return func(options *profileDiscoveryCommandOptions) {
			options.ServiceID = *serviceID
		}
	})
}

func workflowList(bundle profile.Bundle, filter string, serviceFilter string) workflowListReport {
	serviceFilter = strings.TrimSpace(serviceFilter)
	stepCounts := map[string]int{}
	nodes := map[string]profile.InterfaceNode{}
	for _, node := range bundle.InterfaceNodes {
		nodes[node.ID] = node
	}
	serviceIDsByWorkflow := map[string]map[string]bool{}
	matchedStepsByWorkflow := map[string][]workflowServiceMatchStep{}
	for _, item := range bundle.WorkflowBindings {
		workflowID := strings.TrimSpace(item.WorkflowID)
		if workflowID == "" {
			continue
		}
		stepCounts[workflowID]++
		node, ok := nodes[strings.TrimSpace(item.NodeID)]
		if !ok {
			continue
		}
		serviceID := strings.TrimSpace(node.ServiceID)
		if serviceID == "" {
			continue
		}
		if serviceIDsByWorkflow[workflowID] == nil {
			serviceIDsByWorkflow[workflowID] = map[string]bool{}
		}
		serviceIDsByWorkflow[workflowID][serviceID] = true
		if serviceFilter != "" && serviceID == serviceFilter {
			matchedStepsByWorkflow[workflowID] = append(matchedStepsByWorkflow[workflowID], workflowServiceMatchStep{
				StepID:    strings.TrimSpace(item.StepID),
				NodeID:    strings.TrimSpace(item.NodeID),
				CaseID:    strings.TrimSpace(item.CaseID),
				ServiceID: serviceID,
				Required:  item.Required,
				SortOrder: item.SortOrder,
			})
		}
	}
	workflows := append([]profile.Workflow(nil), bundle.Workflows...)
	sort.SliceStable(workflows, func(i, j int) bool {
		return workflows[i].ID < workflows[j].ID
	})
	report := workflowListReport{OK: true, ProfileID: bundle.ID, ServiceID: serviceFilter}
	for _, workflow := range workflows {
		if !matchesDiscoveryFilter(filter, workflow.ID, workflow.DisplayName, workflow.Description) {
			continue
		}
		serviceIDs := sortedWorkflowServiceIDs(serviceIDsByWorkflow[workflow.ID])
		matchedSteps := matchedStepsByWorkflow[workflow.ID]
		if serviceFilter != "" {
			if len(matchedSteps) == 0 {
				continue
			}
			sortWorkflowServiceMatchSteps(matchedSteps)
		}
		report.Items = append(report.Items, workflowListItem{
			ID:           workflow.ID,
			DisplayName:  workflow.DisplayName,
			Description:  workflow.Description,
			StepCount:    stepCounts[workflow.ID],
			ServiceIDs:   serviceIDs,
			MatchedSteps: matchedSteps,
		})
	}
	report.Count = len(report.Items)
	return report
}

func sortedWorkflowServiceIDs(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortWorkflowServiceMatchSteps(items []workflowServiceMatchStep) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		if items[i].StepID != items[j].StepID {
			return items[i].StepID < items[j].StepID
		}
		if items[i].NodeID != items[j].NodeID {
			return items[i].NodeID < items[j].NodeID
		}
		return items[i].CaseID < items[j].CaseID
	})
}
