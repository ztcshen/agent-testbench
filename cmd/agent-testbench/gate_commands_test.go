package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGateBaselineAliasesBaselineCommands(t *testing.T) {
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "gate-baseline.sqlite")

	runCLI(t, "gate", "baseline", "set", "--store", storeRef, "--profile", "profile.local", "--subject", "subject.case", "--status", "passed", "--required")
	out := runCLI(t, "gate", "baseline", "get", "--store", storeRef, "--profile", "profile.local", "--subject", "subject.case")
	for _, want := range []string{"Baseline Gate: profile.local subject.case", "Status: passed", "Required: true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("gate baseline get missing %q:\n%s", want, out)
		}
	}
}
