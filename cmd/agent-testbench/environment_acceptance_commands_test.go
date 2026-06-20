package main

import "testing"

func TestEnvironmentAcceptanceCLIAcceptsLeadingEnvironmentID(t *testing.T) {
	var startPayload map[string]any
	server := newEnvironmentAcceptanceCLIServer(t, &startPayload)
	defer server.Close()

	startOut := runCLI(t, "environment", "acceptance", "start", "env.team",
		"--server-url", server.URL,
		"--request-id", "env-acceptance-001",
		"--base-url", "http://127.0.0.1:18080",
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

func TestCaseBatchCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := newCaseBatchCLIServer(t, &startPayload)
	defer server.Close()

	startOut := runCLI(t, "case", "batch", "start",
		"--server-url", server.URL,
		"--case", "case.alpha",
		"--case", "case.beta",
		"--request-id", "case-batch-001",
		"--base-url", "http://127.0.0.1:18080",
		"--timeout-seconds", "30",
		"--json",
	)
	assertCaseBatchStart(t, decodeCLIJSON[caseBatchStart](t, startOut), startPayload)

	reportOut := runCLI(t, "case", "batch", "report",
		"--server-url", server.URL,
		"--run", "batch.case.001",
		"--json",
	)
	assertCaseBatchReport(t, decodeCLIJSON[caseBatchReport](t, reportOut))
}

func TestEnvironmentAcceptanceCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := newEnvironmentAcceptanceCLIServer(t, &startPayload)
	defer server.Close()

	startOut := runCLI(t, "environment", "acceptance", "start",
		"--server-url", server.URL,
		"--request-id", "env-acceptance-001",
		"--base-url", "http://127.0.0.1:18080",
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
