package main

import (
	"context"
	"errors"
	"fmt"
)

func runEnvironment(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment command")
	}
	switch args[0] {
	case "register":
		return runEnvironmentRegister(ctx, args[1:])
	case "discover":
		return runEnvironmentDiscover(ctx, args[1:])
	case "inspect":
		return runEnvironmentInspect(ctx, args[1:])
	case "bootstrap":
		return runEnvironmentBootstrap(ctx, args[1:])
	case "repo":
		return runEnvironmentRepo(ctx, args[1:])
	case "startup-file":
		return runEnvironmentStartupFile(ctx, args[1:])
	case "components":
		return runEnvironmentComponents(ctx, args[1:])
	case "migration":
		return runEnvironmentMigration(ctx, args[1:])
	case "restore":
		return runEnvironmentRestore(ctx, args[1:])
	case "status":
		return runEnvironmentStatus(ctx, args[1:])
	case "stop":
		return runEnvironmentStop(ctx, args[1:])
	case "acceptance":
		return runEnvironmentAcceptance(ctx, args[1:])
	case "verify":
		return runEnvironmentVerify(ctx, args[1:])
	case "publish-verified":
		return runEnvironmentPublishVerified(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment command: %s", args[0])
	}
}
