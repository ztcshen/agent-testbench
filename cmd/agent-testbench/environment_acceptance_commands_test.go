package main

import (
	"context"
	"strings"
	"testing"
)

func TestEnvironmentAcceptanceCLIRejectsPublicTargetAndEvidenceOverrides(t *testing.T) {
	for _, flag := range []string{"--base-url", "--evidence-dir"} {
		err := runEnvironmentAcceptanceStart(context.Background(), []string{
			"env.team",
			"--server-url", "http://127.0.0.1:1",
			"--request-id", "env-acceptance-001",
			flag, "untrusted-value",
		})
		if err == nil || !strings.Contains(err.Error(), "Store catalog") {
			t.Fatalf("%s error = %v, want Store catalog guidance", flag, err)
		}
	}
}

func TestEnvironmentAcceptanceCLIAcceptsLeadingEnvironmentID(t *testing.T) {
	var startPayload map[string]any
	server := newEnvironmentAcceptanceCLIServer(t, &startPayload)
	defer server.Close()

	startOut := runCLI(t, "environment", "acceptance", "start", "env.team",
		"--server-url", server.URL,
		"--request-id", "env-acceptance-001",
		"--timeout-seconds", "30",
		"--json",
	)
	assertEnvironmentAcceptanceStart(t, decodeCLIJSON[environmentAcceptanceStart](t, startOut), startPayload)

	reportOut := runCLI(t, "environment", "acceptance", "report", "env.team",
		"--server-url", server.URL,
		"--run", "batch.env.acceptance.001",
		"--json",
	)
	assertEnvironmentAcceptanceReport(t, decodeCLIJSON[environmentAcceptanceReport](t, reportOut))
}

func TestEnvironmentAcceptanceCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := newEnvironmentAcceptanceCLIServer(t, &startPayload)
	defer server.Close()

	startOut := runCLI(t, "environment", "acceptance", "start",
		"--server-url", server.URL,
		"--request-id", "env-acceptance-001",
		"--json",
		"env.team",
	)
	assertEnvironmentAcceptanceStart(t, decodeCLIJSON[environmentAcceptanceStart](t, startOut), startPayload)

	reportOut := runCLI(t, "environment", "acceptance", "report",
		"--server-url", server.URL,
		"--run", "batch.env.acceptance.001",
		"--json",
		"env.team",
	)
	assertEnvironmentAcceptanceReport(t, decodeCLIJSON[environmentAcceptanceReport](t, reportOut))
}
