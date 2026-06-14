package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/runner/mysqlruntime"
)

func runRuntime(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing runtime command")
	}
	switch args[0] {
	case "mysql":
		return runRuntimeMySQL(ctx, args[1:])
	default:
		return fmt.Errorf("unknown runtime command: %s", args[0])
	}
}

func runRuntimeMySQL(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing runtime mysql command")
	}
	switch args[0] {
	case "endpoints":
		return runRuntimeMySQLEndpoints(ctx, args[1:])
	default:
		return fmt.Errorf("unknown runtime mysql command: %s", args[0])
	}
}

func runRuntimeMySQLEndpoints(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("runtime mysql endpoints", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	includeTables := flags.Bool("include-tables", false, "Include database and table inventory when the container can be queried")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected runtime mysql endpoints argument: %s", flags.Arg(0))
	}
	report, err := mysqlruntime.DiscoverEndpoints(ctx, *includeTables)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printRuntimeMySQLEndpoints(report)
	return nil
}

func printRuntimeMySQLEndpoints(report mysqlruntime.Report) {
	fmt.Println("MySQL Runtime Endpoints")
	fmt.Printf("Total: %d\n", report.Count)
	for _, item := range report.Items {
		fmt.Printf("- %s: %s\n", item.ContainerName, item.DSN)
		if len(item.Databases) > 0 {
			parts := []string{}
			for _, database := range item.Databases {
				for _, table := range database.Tables {
					parts = append(parts, database.Name+"."+table)
				}
			}
			fmt.Printf("  Tables: %s\n", strings.Join(parts, ", "))
		}
		if len(item.Warnings) > 0 {
			fmt.Printf("  Warnings: %s\n", strings.Join(item.Warnings, "; "))
		}
	}
}
