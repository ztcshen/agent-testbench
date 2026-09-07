package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"agent-testbench/internal/runner/direct"
	"agent-testbench/internal/store"
)

const cliCommandDirect = direct.Mode

func runDirect(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("direct requires register, discover, plan, run, or inspect")
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch args[0] {
	case "register":
		return runDirectRegister(ctx, args[1:])
	case "discover":
		return runDirectDiscover(ctx, args[1:])
	case "plan", "run":
		return runDirectCase(ctx, args[0], args[1:])
	case "inspect":
		return runDirectInspect(ctx, args[1:])
	default:
		return fmt.Errorf("unknown direct command: %s", args[0])
	}
}

type directCLIFlags struct {
	flags *flag.FlagSet
	store *string
	json  *bool
}

func newDirectCLIFlags(name string) directCLIFlags {
	flags := flag.NewFlagSet(cliCommandDirect+" "+name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return directCLIFlags{flags: flags, store: flags.String("store", "", "Named Store or SQL Store DSN"), json: flags.Bool("json", false, "Emit JSON")}
}

func runDirectRegister(ctx context.Context, args []string) error {
	cli := newDirectCLIFlags("register")
	var binding direct.Binding
	cli.flags.StringVar(&binding.ID, "id", "", "Direct execution binding id")
	cli.flags.StringVar(&binding.DisplayName, "display-name", "", "Display name")
	cli.flags.StringVar(&binding.ProjectDir, "project", "", "Existing local test project directory")
	cli.flags.StringVar(&binding.Executable, "executable", "", "Native adapter executable")
	cli.flags.StringVar(&binding.TargetURL, "target-url", "", "Already-running HTTP(S) target")
	cli.flags.StringVar(&binding.EvidenceDir, "evidence-dir", filepath.Join(".runtime", "direct-runs"), "Run state and Evidence root")
	cli.flags.IntVar(&binding.TimeoutSeconds, "timeout-seconds", 60, "Native execution timeout, 1 to 600 seconds")
	expected := cli.flags.Int64("expected-revision", -1, "Expected current catalog revision")
	envNames, targetNames := stringListFlag{}, stringListFlag{}
	cli.flags.Func("arg", "Adapter argument; repeat in execution order", func(value string) error { binding.Args = append(binding.Args, value); return nil })
	cli.flags.Var(&envNames, "env", "Environment variable name to forward; repeat")
	cli.flags.Var(&targetNames, "target-env", "Variable to set to the registered target URL; repeat")
	if err := cli.flags.Parse(args); err != nil {
		return err
	}
	if cli.flags.NArg() != 0 {
		return errors.New("direct register accepts flags only")
	}
	binding.EnvironmentNames, binding.TargetEnvNames = []string(envNames), []string(targetNames)
	request := direct.RegisterRequest{Binding: binding}
	if *expected >= 0 {
		request.ExpectedRevision = expected
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *cli.store, "")
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := direct.Register(ctx, runtime, request)
	if err != nil {
		return err
	}
	if *cli.json {
		return writeIndentedJSON(result)
	}
	fmt.Printf("Direct binding: %s\nCases: %d\nCatalog revision: %d\n", result.Registration.Binding.ID, len(result.Registration.Manifest.Cases), result.Revision)
	return nil
}

func runDirectDiscover(ctx context.Context, args []string) error {
	cli := newDirectCLIFlags("discover")
	if err := cli.flags.Parse(args); err != nil {
		return err
	}
	if cli.flags.NArg() != 0 {
		return errors.New("direct discover accepts flags only")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *cli.store, "")
	if err != nil {
		return err
	}
	defer cleanup()
	items, err := direct.List(ctx, runtime)
	if err != nil {
		return err
	}
	if *cli.json {
		return writeIndentedJSON(map[string]any{"ok": true, "mode": direct.Mode, "bindings": items})
	}
	for _, item := range items {
		for _, test := range item.Registration.Manifest.Cases {
			fmt.Printf("%s\t%s\t%s\n", item.Registration.Binding.ID, test.ID, test.Title)
		}
	}
	return nil
}

func runDirectCase(ctx context.Context, command string, args []string) error {
	cli := newDirectCLIFlags(command)
	var request direct.RunRequest
	cli.flags.StringVar(&request.BindingID, "binding", "", "Registered direct binding id")
	cli.flags.StringVar(&request.CaseID, "case", "", "External case or workflow id")
	cli.flags.StringVar(&request.RunID, "run-id", "", "Idempotent submission identity; a new execution needs a new id")
	dryRun := cli.flags.Bool("dry-run", command == "plan", "Inspect the direct plan without executing the adapter")
	overrides := mapFlag{}
	cli.flags.Var(&overrides, "override", "Case override KEY=VALUE; repeat")
	if err := cli.flags.Parse(args); err != nil {
		return err
	}
	if cli.flags.NArg() != 0 {
		return errors.New("direct execution accepts flags only")
	}
	request.Overrides = overrides.Values()
	runtime, cleanup, err := openRequiredCLIStore(ctx, *cli.store, "")
	if err != nil {
		return err
	}
	defer cleanup()
	if command == "plan" || *dryRun {
		plan, err := direct.BuildPlan(ctx, runtime, request)
		if err != nil {
			return err
		}
		return writeIndentedJSON(plan)
	}
	report, err := direct.Run(ctx, runtime, request)
	if err != nil {
		return err
	}
	if err := writeDirectReport(report, *cli.json); err != nil {
		return err
	}
	return directOutcomeError(report)
}

func directOutcomeError(report direct.Report) error {
	if report.OK && report.Status == store.StatusPassed {
		return nil
	}
	return fmt.Errorf("direct run did not pass: status=%s category=%s %s; inspect run %s", report.Status, report.FailureCategory, report.Error, report.RunID)
}

func runDirectInspect(ctx context.Context, args []string) error {
	cli := newDirectCLIFlags("inspect")
	id := cli.flags.String("run", "", "Direct run id")
	if err := cli.flags.Parse(args); err != nil {
		return err
	}
	if *id == "" || cli.flags.NArg() != 0 {
		return errors.New("direct inspect requires --run ID")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *cli.store, "")
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := direct.Inspect(ctx, runtime, *id)
	if err != nil {
		return err
	}
	return writeDirectReport(report, *cli.json)
}

func writeDirectReport(report direct.Report, jsonOutput bool) error {
	if jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Mode: %s\nRun: %s\nCase: %s\nStatus: %s\nEvidence: %s\n", report.Mode, report.RunID, report.CaseID, report.Status, report.EvidenceDir)
	return nil
}
