package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	caseInspectViewDiagnose = "diagnose"
	caseInspectViewEvidence = builtInTaskStepEvidence
	caseInspectViewRuns     = "runs"
	caseInspectViewTiming   = "timing"
	caseInspectFlagView     = "--view"
)

func runCaseInspect(ctx context.Context, args []string) error {
	view, rest, err := parseCaseInspectView(args)
	if err != nil {
		return err
	}
	switch view {
	case "", caseInspectViewDiagnose:
		return runCaseDiagnose(ctx, rest)
	case caseInspectViewEvidence:
		return runCaseEvidence(ctx, rest)
	case caseInspectViewRuns:
		return runCaseRuns(ctx, rest)
	case caseInspectViewTiming:
		return runCaseTiming(ctx, rest)
	default:
		return fmt.Errorf("unknown case inspect view: %s", view)
	}
}

func parseCaseInspectView(args []string) (string, []string, error) {
	view := caseInspectViewDiagnose
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == caseInspectFlagView {
			if i+1 >= len(args) {
				return "", nil, errors.New(caseInspectFlagView + " requires a value")
			}
			view = strings.TrimSpace(args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, caseInspectFlagView+"=") {
			view = strings.TrimSpace(strings.TrimPrefix(arg, caseInspectFlagView+"="))
			continue
		}
		rest = append(rest, arg)
	}
	return view, rest, nil
}
