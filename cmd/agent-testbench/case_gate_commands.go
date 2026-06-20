package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/store"
)

type caseGateReport struct {
	OK              bool              `json:"ok"`
	RunID           string            `json:"runId,omitempty"`
	Counts          caseGateCounts    `json:"counts"`
	Gates           caseGateGates     `json:"gates"`
	FailedCaseRuns  []caseRunsCLIItem `json:"failedCaseRuns"`
	MissingEvidence []caseRunsCLIItem `json:"missingEvidence"`
	NextActions     []string          `json:"nextActions"`
	Warnings        []string          `json:"warnings"`
}

type caseGateCounts struct {
	Total            int `json:"total"`
	Passed           int `json:"passed"`
	Failed           int `json:"failed"`
	Other            int `json:"other"`
	EvidenceComplete int `json:"evidenceComplete"`
}

type caseGateGates struct {
	HasCaseRuns      bool `json:"hasCaseRuns"`
	NoFailures       bool `json:"noFailures"`
	MinPassed        bool `json:"minPassed"`
	EvidenceComplete bool `json:"evidenceComplete"`
}

type caseGateOptions struct {
	RunID             string
	RequireNoFailures bool
	RequireEvidence   bool
	MinPassed         int
}

func runCaseGate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runFilter := flags.String("run", "", "Only gate case runs for one run id")
	requireNoFailures := flags.Bool("require-no-failures", false, "Fail when any selected case run is not passed")
	requireEvidence := flags.Bool("require-evidence", false, "Fail when any selected case run has no indexed Evidence")
	minPassed := flags.Int("min-passed", 0, "Fail unless at least this many selected case runs passed")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := buildCaseGateReport(ctx, runtime, caseGateOptions{
		RunID:             *runFilter,
		RequireNoFailures: *requireNoFailures,
		RequireEvidence:   *requireEvidence,
		MinPassed:         *minPassed,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printCaseGate(report)
	}
	if !report.OK {
		return errors.New("case gate failed")
	}
	return nil
}

func buildCaseGateReport(ctx context.Context, runtime store.Store, options caseGateOptions) (caseGateReport, error) {
	items, err := listCaseRunsFromStore(ctx, runtime, options.RunID)
	if err != nil {
		return caseGateReport{}, err
	}
	report := caseGateReport{
		RunID:           strings.TrimSpace(options.RunID),
		FailedCaseRuns:  []caseRunsCLIItem{},
		MissingEvidence: []caseRunsCLIItem{},
		NextActions:     []string{},
		Warnings:        append([]string(nil), items.Warnings...),
	}
	for _, item := range items.CaseRuns {
		report.Counts.Total++
		if strings.EqualFold(item.Status, store.StatusPassed) {
			report.Counts.Passed++
		} else if strings.EqualFold(item.Status, store.StatusFailed) {
			report.Counts.Failed++
			report.FailedCaseRuns = append(report.FailedCaseRuns, item)
		} else {
			report.Counts.Other++
			report.FailedCaseRuns = append(report.FailedCaseRuns, item)
		}
		if item.EvidenceCount > 0 {
			report.Counts.EvidenceComplete++
		} else {
			report.MissingEvidence = append(report.MissingEvidence, item)
		}
	}
	report.Gates = caseGateGates{
		HasCaseRuns:      report.Counts.Total > 0,
		NoFailures:       report.Counts.Failed == 0 && report.Counts.Other == 0,
		MinPassed:        report.Counts.Passed >= options.MinPassed,
		EvidenceComplete: len(report.MissingEvidence) == 0,
	}
	report.OK = report.Gates.HasCaseRuns &&
		(!options.RequireNoFailures || report.Gates.NoFailures) &&
		(!options.RequireEvidence || report.Gates.EvidenceComplete) &&
		report.Gates.MinPassed
	report.NextActions = caseGateNextActions(report, options)
	return report, nil
}

func caseGateNextActions(report caseGateReport, options caseGateOptions) []string {
	actions := []string{}
	if !report.Gates.HasCaseRuns {
		base := "agent-testbench case inspect --view runs --json"
		if report.RunID != "" {
			base = "agent-testbench case inspect --view runs --run " + report.RunID + " --json"
		}
		return []string{base}
	}
	for index, item := range report.FailedCaseRuns {
		if index >= 3 {
			break
		}
		actions = append(actions, "agent-testbench case diagnose --case-run "+item.ID+" --json")
	}
	if options.RequireEvidence {
		for index, item := range report.MissingEvidence {
			if index >= 3 {
				break
			}
			actions = append(actions, "agent-testbench case inspect --view evidence --case-run "+item.ID+" --json")
		}
	}
	if options.MinPassed > 0 && !report.Gates.MinPassed {
		actions = append(actions, fmt.Sprintf("Run or repair enough cases to reach min-passed=%d", options.MinPassed))
	}
	if len(actions) == 0 {
		actions = append(actions, "Case gate passed; no action needed")
	}
	return actions
}

func printCaseGate(report caseGateReport) {
	fmt.Println("Case Gate")
	fmt.Printf("OK: %t\n", report.OK)
	if report.RunID != "" {
		fmt.Printf("Run: %s\n", report.RunID)
	}
	fmt.Printf("Total: %d Passed: %d Failed: %d Other: %d EvidenceComplete: %d\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed, report.Counts.Other, report.Counts.EvidenceComplete)
	fmt.Printf("Gates: hasCaseRuns=%t noFailures=%t minPassed=%t evidenceComplete=%t\n", report.Gates.HasCaseRuns, report.Gates.NoFailures, report.Gates.MinPassed, report.Gates.EvidenceComplete)
	for _, item := range report.FailedCaseRuns {
		fmt.Printf("Failed: %s %s %s\n", item.ID, item.CaseID, item.Status)
	}
	for _, item := range report.MissingEvidence {
		fmt.Printf("Missing Evidence: %s %s\n", item.ID, item.CaseID)
	}
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}
