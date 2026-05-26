package main

import (
	"context"
	"flag"
	"os"

	"agent-testbench/internal/store"
)

type caseEvidenceCLIFlags struct {
	flags     *flag.FlagSet
	storeRef  *string
	storeURL  *string
	caseRunID *string
	runID     *string
	caseID    *string
	stepID    *string
	json      *bool
}

func newCaseEvidenceCLIFlags(commandName string) caseEvidenceCLIFlags {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	out := caseEvidenceCLIFlags{flags: flags}
	out.storeRef = flags.String("store", "", "Named Store config or Store DSN")
	out.storeURL = flags.String("store-url", "", legacyStoreURLFlagHelp)
	out.caseRunID = flags.String("case-run", "", "Case run id")
	out.runID = flags.String("run", "", "Run id")
	out.caseID = flags.String("case-id", "", "Case id within the run")
	out.stepID = flags.String("step-id", "", "Workflow step id within the run")
	out.json = flags.Bool("json", false, "Emit a machine-readable JSON report")
	return out
}

func (f caseEvidenceCLIFlags) parse(args []string) error {
	return f.flags.Parse(args)
}

func (f caseEvidenceCLIFlags) openStore(ctx context.Context) (store.Store, func(), error) {
	return openRequiredCLIStore(ctx, *f.storeRef, *f.storeURL)
}
