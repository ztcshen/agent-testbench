package controlplane

import (
	"strings"
	"testing"
)

func TestJoinCaseURLKeepsConfiguredTargetOrigin(t *testing.T) {
	got, err := joinCaseURL("http://127.0.0.1:8080/root/", "/v1/items", map[string]any{"id": "123"})
	if err != nil {
		t.Fatalf("join case url: %v", err)
	}
	if want := "http://127.0.0.1:8080/v1/items?id=123"; got != want {
		t.Fatalf("joined url = %q, want %q", got, want)
	}
}

func TestJoinCaseURLRejectsPathsThatOverrideConfiguredTargetOrigin(t *testing.T) {
	for _, path := range []string{
		"http://attacker.invalid/collect",
		"//attacker.invalid/collect",
		"http:attacker.invalid/collect",
	} {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			if _, err := joinCaseURL("http://127.0.0.1:8080", path, nil); err == nil {
				t.Fatalf("joinCaseURL(%q) succeeded, want origin override rejection", path)
			}
		})
	}
}

func TestValidateTrustedTestKitRunIDRejectsUnsafePathSegments(t *testing.T) {
	for _, runID := range []string{".", "..", "../escape", `..\\escape`, "/tmp/escape", `C:\\tmp\\escape`, "run:escape"} {
		t.Run(strings.ReplaceAll(runID, "/", "_"), func(t *testing.T) {
			if err := validateTrustedTestKitRunID(runID); err == nil {
				t.Fatalf("validateTrustedTestKitRunID(%q) succeeded, want rejection", runID)
			}
		})
	}
	for _, runID := range []string{"run-001", "run.20260716T120000Z", "map_task_01"} {
		if err := validateTrustedTestKitRunID(runID); err != nil {
			t.Fatalf("validateTrustedTestKitRunID(%q): %v", runID, err)
		}
	}
}
