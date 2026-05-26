package main

import (
	"reflect"
	"testing"
)

func TestCaseSelectionCLIFlagsKeepsTagState(t *testing.T) {
	selection := newCaseSelectionCLIFlags("case test", "active")
	if err := selection.parse([]string{"--tag", "smoke", "--tag", "regression"}); err != nil {
		t.Fatalf("parse case selection flags: %v", err)
	}

	got := selection.caseListFilter().Tags
	want := []string{"smoke", "regression"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
}

func TestCaseSuiteImpactFlagsKeepRepeatedState(t *testing.T) {
	selection := newCaseSelectionCLIFlags("case impact test", "active")
	impact := addCaseSuiteImpactFlags(selection, "base URL", 3, "timeout")
	if err := selection.parse([]string{"--signal", "/alpha", "--change", "variant", "--action", "run", "--action", "rerun"}); err != nil {
		t.Fatalf("parse case impact flags: %v", err)
	}

	if got, want := impact.signalValues(), []string{"/alpha", "variant"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("signals = %#v, want %#v", got, want)
	}
	if got, want := impact.planOptions("").Actions, []string{"run", "rerun"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %#v, want %#v", got, want)
	}
}

func TestCaseSuiteBatchRequestFlagsKeepRepeatedState(t *testing.T) {
	selection := newCaseSelectionCLIFlags("case batch test", "active")
	batch := addCaseSuiteBatchRequestFlags(selection)
	if err := selection.parse([]string{"--signal", "/alpha", "--change", "variant"}); err != nil {
		t.Fatalf("parse case batch flags: %v", err)
	}

	if got, want := batch.signalValues(), []string{"/alpha", "variant"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("signals = %#v, want %#v", got, want)
	}
}
