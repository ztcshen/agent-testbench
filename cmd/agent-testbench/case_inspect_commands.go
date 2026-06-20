package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func runCaseInspect(ctx context.Context, args []string) error {
	view, rest, err := parseCaseInspectView(args)
	if err != nil {
		return err
	}
	switch view {
	case "", "diagnose":
		return runCaseDiagnose(ctx, rest)
	case "evidence":
		return runCaseEvidence(ctx, rest)
	case "runs":
		return runCaseRuns(ctx, rest)
	case "timing":
		return runCaseTiming(ctx, rest)
	default:
		return fmt.Errorf("unknown case inspect view: %s", view)
	}
}

func parseCaseInspectView(args []string) (string, []string, error) {
	view := "diagnose"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--view" {
			if i+1 >= len(args) {
				return "", nil, errors.New("--view requires a value")
			}
			view = strings.TrimSpace(args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, "--view=") {
			view = strings.TrimSpace(strings.TrimPrefix(arg, "--view="))
			continue
		}
		rest = append(rest, arg)
	}
	return view, rest, nil
}
