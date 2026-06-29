package main

import (
	"context"
	"fmt"
)

func runCase(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return printCommandHelp([]string{"case"})
	}
	switch args[0] {
	case "discover":
		return runCaseDiscover(ctx, args[1:])
	case "catalog":
		return runCaseCatalog(ctx, args[1:])
	case "suite":
		return runCaseSuite(ctx, args[1:])
	case "run":
		return runCaseRun(ctx, args[1:])
	case "inspect":
		return runCaseInspect(ctx, args[1:])
	case "runs":
		return runCaseRuns(ctx, args[1:])
	case "evidence":
		return runCaseEvidence(ctx, args[1:])
	case "diagnose":
		return runCaseDiagnose(ctx, args[1:])
	case "gate":
		return runCaseGate(ctx, args[1:])
	case "config":
		return runCaseConfig(ctx, args[1:])
	case "timing":
		return runCaseTiming(ctx, args[1:])
	case "incomplete-batches":
		return runCaseIncompleteBatches(ctx, args[1:])
	default:
		return fmt.Errorf("unknown case command: %s", args[0])
	}
}
