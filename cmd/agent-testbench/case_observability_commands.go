package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runCaseTiming(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case timing", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	kind := flags.String("kind", "", "Timing kind")
	maxAgeMinutes := flags.String("max-age-minutes", "", "Only include case runs created within this many minutes")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, err := controlplane.CaseTimingPayload(ctx, runtime, *kind, *maxAgeMinutes)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	printCaseTiming(payload)
	return nil
}

func printCaseTiming(payload map[string]any) {
	summary := mapFromReportAny(payload["summary"])
	fmt.Println("Case Timing")
	fmt.Printf("Case Runs: %s\n", valueString(summary["caseRunCount"]))
	fmt.Printf("Measured: %s\n", valueString(summary["durationMeasuredCount"]))
	fmt.Printf("Max Duration: %s ms\n", valueString(summary["maxDurationMs"]))
	if slowest := mapFromReportAny(summary["slowestRows"]); len(slowest) > 0 {
		if row := mapFromReportAny(slowest["caseRun"]); len(row) > 0 {
			fmt.Printf("Slowest: %s %s ms\n", valueString(row["id"]), valueString(row["durationMs"]))
		}
	}
}

func runCaseEvidence(ctx context.Context, args []string) error {
	selection := newCaseEvidenceCLIFlags("case evidence")
	if err := selection.parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := selection.openStore(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, err := readCaseEvidence(ctx, runtime, *selection.caseRunID, *selection.runID, *selection.caseID, *selection.stepID)
	if err != nil {
		return err
	}
	if *selection.json {
		return writeIndentedJSON(payload)
	}
	printCaseEvidence(payload)
	return nil
}

func readCaseEvidence(ctx context.Context, runtime store.Store, caseRunID string, runID string, caseID string, stepID string) (map[string]any, error) {
	var payload map[string]any
	var ok bool
	var err error
	if strings.TrimSpace(caseRunID) != "" {
		payload, ok, err = controlplane.CaseEvidencePayloadForCaseRunID(ctx, runtime, caseRunID)
	} else if strings.TrimSpace(runID) != "" {
		payload, ok, err = controlplane.CaseEvidencePayloadForRunID(ctx, runtime, runID, caseID, stepID)
	} else {
		return nil, errors.New("--case-run or --run is required")
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("case evidence not found")
	}
	return payload, nil
}

func printCaseEvidence(payload map[string]any) {
	evidence := mapFromReportAny(payload["evidence"])
	summary := mapFromReportAny(evidence["summary"])
	fmt.Println("Case Evidence")
	fmt.Printf("Case Run: %s\n", valueString(summary["case_run_id"]))
	fmt.Printf("Case: %s\n", valueString(summary["case_id"]))
	fmt.Printf("Run: %s\n", valueString(summary["run_id"]))
	fmt.Printf("Status: %s\n", valueString(summary["status"]))
	fmt.Printf("Operation: %s\n", valueString(summary["operation"]))
	if evidencePath := valueString(summary["evidence_path"]); evidencePath != "" {
		fmt.Printf("Evidence: %s\n", evidencePath)
	}
}
