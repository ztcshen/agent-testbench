package apicasecommand

import (
	"testing"

	"agent-testbench/internal/domain/profile"
)

func TestSuggestedRunCommandIncludesOptionalFlags(t *testing.T) {
	command := SuggestedRunCommand(profile.APICase{
		CasePath:    "cases/create item.json",
		BaseURL:     "http://127.0.0.1:8080",
		EvidenceDir: "evidence/nightly run",
	})

	want := `agent-testbench case run --case "cases/create item.json" --base-url "http://127.0.0.1:8080" --evidence-dir "evidence/nightly run"`
	if command != want {
		t.Fatalf("suggested command = %q, want %q", command, want)
	}
}

func TestSuggestedRunCommandForProfileIncludesNonDefaultProfile(t *testing.T) {
	command := SuggestedRunCommandForProfile(profile.APICase{
		CasePath: "cases/create item.json",
	}, "sample")

	want := `agent-testbench case run --case "cases/create item.json" --profile "sample"`
	if command != want {
		t.Fatalf("suggested command = %q, want %q", command, want)
	}
}

func TestSuggestedRunCommandForProfileOmitsDefaultProfile(t *testing.T) {
	command := SuggestedRunCommandForProfile(profile.APICase{
		CasePath: "cases/create item.json",
	}, "default")

	want := `agent-testbench case run --case "cases/create item.json"`
	if command != want {
		t.Fatalf("suggested command = %q, want %q", command, want)
	}
}

func TestSuggestedRunCommandRequiresCasePath(t *testing.T) {
	if command := SuggestedRunCommand(profile.APICase{BaseURL: "http://127.0.0.1:8080"}); command != "" {
		t.Fatalf("suggested command without case path = %q", command)
	}
}
