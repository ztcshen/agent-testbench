package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

const environmentStopActionComposeStop = "compose-stop"

type environmentStopOptions struct {
	environmentLifecycleOptions
	Down          bool
	RemoveOrphans bool
}

type environmentStopReport struct {
	OK          bool                        `json:"ok"`
	Environment map[string]any              `json:"environment"`
	Docker      environmentStopDockerReport `json:"docker"`
	Error       string                      `json:"error,omitempty"`
}

type environmentStopDockerReport struct {
	OK      bool                                          `json:"ok"`
	Action  string                                        `json:"action"`
	Linkage *environmentRestoreDockerCleanupLinkageReport `json:"linkage,omitempty"`
	Command []string                                      `json:"command,omitempty"`
	Output  string                                        `json:"output,omitempty"`
	Error   string                                        `json:"error,omitempty"`
}

func runEnvironmentStop(ctx context.Context, args []string) error {
	options, err := parseEnvironmentStopOptions(args)
	if err != nil {
		return err
	}
	runtime, env, graph, plan, cleanup, err := loadEnvironmentLifecyclePlan(ctx, options.environmentLifecycleOptions)
	if err != nil {
		return err
	}
	defer cleanup()
	report := environmentStopReport{OK: true}
	report.Docker = environmentStopDocker(ctx, plan.Compose, graph, plan.Workspace, options)
	if !report.Docker.OK {
		report.OK = false
		report.Error = report.Docker.Error
	}
	persisted, persistErr := persistEnvironmentStopSummary(ctx, runtime, env, report, plan.Workspace)
	if persistErr != nil {
		report.OK = false
		report.Error = persistErr.Error()
	} else {
		env = persisted
	}
	report.Environment = environmentPayload(env)
	if options.JSONOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printEnvironmentStopReport(report)
	}
	if !report.OK {
		return errors.New("environment stop did not complete")
	}
	return nil
}

func parseEnvironmentStopOptions(args []string) (environmentStopOptions, error) {
	flags := flag.NewFlagSet("environment stop", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workspace := flags.String("workspace", "", "Local workspace for generated compose artifacts")
	down := flags.Bool("down", false, "Use docker compose down instead of docker compose stop")
	removeOrphans := flags.Bool("remove-orphans", false, "Include --remove-orphans with --down")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentStopOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentStopOptions{}, errors.New("environment id is required")
	}
	if strings.TrimSpace(*workspace) == "" {
		return environmentStopOptions{}, errors.New("--workspace is required")
	}
	if *removeOrphans && !*down {
		return environmentStopOptions{}, errors.New("--remove-orphans requires --down")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return environmentStopOptions{}, err
	}
	return environmentStopOptions{
		environmentLifecycleOptions: environmentLifecycleOptions{
			EnvironmentID: id,
			StoreRef:      *storeRef,
			StoreURL:      resolvedStoreURL,
			Workspace:     *workspace,
			JSONOutput:    *jsonOutput,
		},
		Down:          *down,
		RemoveOrphans: *removeOrphans,
	}, nil
}

func environmentStopDocker(ctx context.Context, compose map[string]any, graph store.EnvironmentComponentGraph, workspace string, options environmentStopOptions) environmentStopDockerReport {
	statusReport := environmentStatusDockerReport{OK: true}
	if !prepareEnvironmentLifecycleComposeFiles(&statusReport, compose, workspace) {
		return environmentStopDockerReport{OK: false, Action: statusReport.Action, Error: statusReport.Error}
	}
	composeFiles := environmentRestoreComposeFiles(compose)
	if len(composeFiles) == 0 {
		return environmentStopDockerReport{OK: false, Action: "no-compose-plan", Error: "environment stop requires a recorded composeFile"}
	}
	composeBaseArgs := environmentRestoreComposeBaseArgs(compose, workspace, environmentRestoreResolvedComposeFiles(workspace, composeFiles))
	command := append([]string{"docker", "compose"}, composeBaseArgs...)
	action := environmentStopActionComposeStop
	var downLinkage *environmentRestoreDockerCleanupLinkageReport
	if options.Down {
		linkage := environmentRestoreDockerCleanupLinkage(compose, graph, workspace, composeFiles)
		downLinkage = &linkage
		if !linkage.OK {
			return environmentStopDockerReport{
				OK:      false,
				Action:  "compose-down-blocked",
				Linkage: downLinkage,
				Error:   firstNonEmpty(linkage.Error, "Docker Compose down requires complete Store-to-Compose environment linkage"),
			}
		}
		action = "compose-down"
		command = append(command, "down")
		if options.RemoveOrphans {
			command = append(command, "--remove-orphans")
		}
	} else {
		services := environmentLifecycleComposeServices(compose, workspace)
		if len(services) == 0 {
			return environmentStopDockerReport{OK: false, Action: environmentStopActionComposeStop, Error: "environment stop found no compose services; record compose services or provide a compose file with services"}
		}
		command = append(command, "stop")
		command = append(command, services...)
	}
	output, errText := runRestoreCommand(ctx, workspace, command)
	report := environmentStopDockerReport{OK: errText == "", Action: action, Linkage: downLinkage, Command: command, Output: output}
	if errText != "" {
		report.Error = errText
	}
	return report
}

func persistEnvironmentStopSummary(ctx context.Context, runtime store.Store, env store.Environment, report environmentStopReport, workspace string) (store.Environment, error) {
	summary := jsonObjectString(env.SummaryJSON)
	summary["lastStop"] = map[string]any{
		"attemptedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"ok":          report.Docker.OK,
		"action":      report.Docker.Action,
		"workspace":   workspace,
		"command":     report.Docker.Command,
		"error":       report.Docker.Error,
	}
	env.SummaryJSON = mustCompactJSON(summary)
	env.UpdatedAt = time.Now().UTC()
	return runtime.UpsertEnvironment(ctx, env)
}

func printEnvironmentStopReport(report environmentStopReport) {
	fmt.Printf("Environment Stop: %s\n", valueString(report.Environment["id"]))
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Docker: %s ok=%t\n", report.Docker.Action, report.Docker.OK)
	if len(report.Docker.Command) > 0 {
		fmt.Printf("Command: %s\n", strings.Join(report.Docker.Command, " "))
	}
	if report.Docker.Linkage != nil {
		for _, item := range report.Docker.Linkage.RepairPlan {
			fmt.Printf("Repair: %s -> %s\n", item.Name, item.Action)
			if len(item.Missing) > 0 {
				fmt.Printf("  missing: %s\n", strings.Join(item.Missing, ", "))
			}
			if item.CommandHint != "" {
				fmt.Printf("  hint: %s\n", item.CommandHint)
			}
		}
	}
	if report.Error != "" {
		fmt.Printf("Error: %s\n", report.Error)
	}
}
