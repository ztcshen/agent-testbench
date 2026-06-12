package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
)

type interfaceNodeListReport struct {
	OK        bool                    `json:"ok"`
	ProfileID string                  `json:"profileId"`
	Count     int                     `json:"count"`
	Items     []interfaceNodeListItem `json:"items"`
}

type interfaceNodeListItem struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	ServiceID   string `json:"serviceId,omitempty"`
	CaseCount   int    `json:"caseCount"`
}

func runInterfaceNode(args []string) error {
	if len(args) == 0 {
		return errors.New("missing interface-node command")
	}
	if args[0] == "discover" {
		return runInterfaceNodeDiscover(context.Background(), args[1:])
	}
	if args[0] == "coverage" {
		return runInterfaceNodeCoverage(context.Background(), args[1:], false)
	}
	if args[0] == "coverage-gaps" {
		return runInterfaceNodeCoverage(context.Background(), args[1:], true)
	}
	if args[0] != "case" {
		return fmt.Errorf("unknown interface-node command: %s", args[0])
	}
	if len(args) < 2 {
		return errors.New("missing interface-node case command")
	}
	switch args[1] {
	case "audit":
		return runInterfaceNodeCaseAudit(args[2:])
	case "draft":
		return runInterfaceNodeCaseDraft(args[2:])
	case "apply":
		return runInterfaceNodeCaseApply(args[2:])
	case "report":
		return runInterfaceNodeCaseReport(context.Background(), args[2:])
	default:
		return fmt.Errorf("unknown interface-node case command: %s", args[1])
	}
}

func runInterfaceNodeDiscover(ctx context.Context, args []string) error {
	options, err := parseProfileDiscoveryCommandOptions("interface-node discover", "Filter by id, display name, or operation", args)
	if err != nil {
		return err
	}
	bundle, cleanup, err := options.loadDiscoveryBundle(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	report := interfaceNodeList(bundle, options.Filter)
	if options.JSONOutput {
		return writeIndentedJSON(report)
	}
	for _, item := range report.Items {
		fmt.Printf("%s\t%s\t%d\n", item.ID, item.DisplayName, item.CaseCount)
	}
	return nil
}

func runInterfaceNodeCoverage(ctx context.Context, args []string, gapsOnly bool) error {
	name := "interface-node coverage"
	if gapsOnly {
		name = "interface-node coverage-gaps"
	}
	options, err := parseProfileWorkflowStoreCommandOptions(name, args, false)
	if err != nil {
		return err
	}
	bundle, runtime, _, cleanup, err := options.loadRequiredBundle(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	var payload map[string]any
	if gapsOnly {
		payload, err = controlplane.InterfaceNodeCoverageGapsPayload(ctx, bundle, options.WorkflowID, runtime)
	} else {
		payload, err = controlplane.InterfaceNodeCoveragePayload(ctx, bundle, options.WorkflowID, runtime)
	}
	if err != nil {
		return err
	}
	if options.JSONOutput {
		return writeIndentedJSON(payload)
	}
	printInterfaceNodeCoverage(payload, gapsOnly)
	return nil
}

func interfaceNodeList(bundle profile.Bundle, filter string) interfaceNodeListReport {
	caseCounts := map[string]int{}
	for _, item := range bundle.APICases {
		if strings.TrimSpace(item.NodeID) != "" {
			caseCounts[item.NodeID]++
		}
	}
	nodes := append([]profile.InterfaceNode(nil), bundle.InterfaceNodes...)
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].SortOrder != nodes[j].SortOrder {
			return nodes[i].SortOrder < nodes[j].SortOrder
		}
		return nodes[i].ID < nodes[j].ID
	})
	report := interfaceNodeListReport{OK: true, ProfileID: bundle.ID}
	for _, node := range nodes {
		if !matchesDiscoveryFilter(filter, node.ID, node.DisplayName, node.Operation) {
			continue
		}
		report.Items = append(report.Items, interfaceNodeListItem{
			ID:          node.ID,
			DisplayName: node.DisplayName,
			Operation:   node.Operation,
			Method:      node.Method,
			Path:        node.Path,
			ServiceID:   node.ServiceID,
			CaseCount:   caseCounts[node.ID],
		})
	}
	report.Count = len(report.Items)
	return report
}

func printInterfaceNodeCoverage(payload map[string]any, gapsOnly bool) {
	if gapsOnly {
		fmt.Printf("Interface Node Coverage Gaps: %s\n", valueString(payload["workflowId"]))
		summary := mapFromReportAny(payload["summary"])
		fmt.Printf("Total Steps: %d\n", intFromReportAny(summary["totalSteps"]))
		fmt.Printf("Gaps: %d\n", intFromReportAny(summary["gapCount"]))
		for _, item := range listFromReportAny(payload["gaps"]) {
			row := mapFromReportAny(item)
			fmt.Printf("Gap: %s Node: %s Case: %s\n", valueString(row["stepId"]), valueString(row["nodeId"]), valueString(row["caseId"]))
		}
		return
	}
	fmt.Printf("Interface Node Coverage: %s\n", valueString(payload["workflowId"]))
	summary := mapFromReportAny(payload["summary"])
	fmt.Printf("Total Steps: %d\n", intFromReportAny(summary["totalSteps"]))
	fmt.Printf("Mapped Steps: %d\n", intFromReportAny(summary["mappedSteps"]))
	fmt.Printf("Unmapped Steps: %d\n", intFromReportAny(summary["unmappedSteps"]))
	for _, item := range listFromReportAny(payload["rows"]) {
		row := mapFromReportAny(item)
		fmt.Printf("Step: %s Node: %s Mapped: %t Admission: %s\n", valueString(row["stepId"]), valueString(row["nodeId"]), boolFromReportAny(row["mapped"]), valueString(row["admissionStatus"]))
	}
}
