package main

import (
	"context"
	"errors"
	"fmt"
)

func runGate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing gate command")
	}
	switch args[0] {
	case "baseline":
		return runBaseline(ctx, args[1:])
	default:
		return fmt.Errorf("unknown gate command: %s", args[0])
	}
}
