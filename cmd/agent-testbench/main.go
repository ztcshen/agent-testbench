package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/casesuite"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("AgentTestBench %s\n", version)
	case "help", "--help", "-h":
		printHelp()
	case "commands":
		if err := runCommands(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "store":
		if err := runStore(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "sandbox":
		if err := runSandbox(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "environment":
		if err := runEnvironment(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "runtime":
		if err := runRuntime(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "profile":
		if err := runProfile(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "template-package", "template-packages":
		if err := runTemplatePackage(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "config":
		if err := runConfig(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "evidence":
		if err := runEvidence(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "trace":
		if err := runTrace(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "replay":
		if err := runReplay(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "executor":
		if err := runExecutor(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "workflow":
		if err := runWorkflow(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "baseline":
		if err := runBaseline(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "template":
		if err := runTemplate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "case":
		if err := runCase(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "interface-node":
		if err := runInterfaceNode(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Println(helpText())
}

func helpText() string {
	return `AgentTestBench

Usage:
  agent-testbench version
  agent-testbench commands [--filter TEXT] [--json]
  agent-testbench store config set NAME --url postgres://...
  agent-testbench store config set NAME --url mysql://...
  agent-testbench store config set NAME --url sqlite://PATH
  agent-testbench store config list [--json]
  agent-testbench store use NAME
  agent-testbench store current [--json]
  agent-testbench store status [--store NAME_OR_DSN] [--json]
  agent-testbench store provision [--store NAME_OR_DSN] [--json]
  agent-testbench store upgrade [--store NAME_OR_DSN]
  agent-testbench store ddl [--backend postgres|mysql] [--store NAME_OR_DSN]
  agent-testbench store copy --from NAME_OR_DSN --to NAME_OR_DSN [--require-environment ENV_ID] [--require-verification-workflow ID] [--require-verified-environment] [--require-min-components N] [--require-min-dependencies N] [--require-min-assets N] [--require-inline-asset-bytes N] [--json]
  agent-testbench environment register --id ID [--store NAME_OR_DSN] [--display-name NAME] [--service ID] [--repo SERVICE=PATH] [--branch SERVICE=BRANCH] [--checkout SERVICE=PATH] [--package-repo URL] [--package-branch BRANCH] [--package-ref REF] [--compose-file PATH]... [--compose-generated-file TARGET=SOURCE_FILE]... [--compose-env KEY=VALUE]... [--start-command TEXT] [--health-url URL] [--health-tcp HOST:PORT] [--health-command CMD] [--health-compose-service SERVICE] [--verification-workflow ID] [--json]
  agent-testbench environment discover [--store NAME_OR_DSN] [--all] [--json]
  agent-testbench environment inspect ENV_ID [--store NAME_OR_DSN] [--json]
  agent-testbench environment bootstrap ENV_ID [--store NAME_OR_DSN] [--json]
  agent-testbench environment repo set ENV_ID [--repo SERVICE=URL] [--branch SERVICE=BRANCH] [--repo-ref SERVICE=REF] [--checkout SERVICE=PATH] [--store NAME_OR_DSN] [--json]
  agent-testbench environment startup-file put ENV_ID --file TARGET=SOURCE_FILE [--store NAME_OR_DSN] [--json]
  agent-testbench environment components inspect ENV_ID [--store NAME_OR_DSN] [--json]
  agent-testbench environment components replace ENV_ID --file COMPONENT_GRAPH_JSON [--store NAME_OR_DSN] [--json]
  agent-testbench environment restore ENV_ID --workspace PATH [--store NAME_OR_DSN] [--execute] [--pull] [--prepare-repos-only] [--assume-clean-docker] [--use-existing-containers] [--clean-docker-state] [--clean-docker-images] [--allow-destructive-docker-cleanup] [--run-workflow --server-url URL] [--base-url URL] [--workflow-output-dir PATH] [--health-timeout-seconds N] [--json]
  agent-testbench environment acceptance start ENV_ID --server-url URL --request-id ID [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench environment acceptance report ENV_ID --server-url URL --run ID [--json]
  agent-testbench environment verify ENV_ID --run ID --status STATUS [--evidence-complete] [--topology-complete] [--store NAME_OR_DSN] [--json]
  agent-testbench environment publish-verified ENV_ID [--store NAME_OR_DSN] [--json]
  agent-testbench runtime mysql endpoints [--include-tables] [--json]
  agent-testbench sandbox start [--store NAME_OR_DSN] [--service ID] [--kind KIND] [--timeout-seconds N] [--json]
  agent-testbench sandbox service register --id ID [--store NAME_OR_DSN] [--display-name NAME] [--kind KIND] [--service-port N] [--health-url URL] [--json]
  agent-testbench sandbox interface register --id ID --service-id ID --path PATH [--store NAME_OR_DSN] [--method METHOD] [--case-id ID] [--case-title TEXT] [--required-for-admission] [--json]
  agent-testbench template-package install --from PATH [--profile-home PATH] [--force]
  agent-testbench template-package inspect --template-package PATH_OR_ID [--profile-home PATH]
  agent-testbench template-package catalog-index [--store NAME_OR_DSN] [--json]
  agent-testbench template-package verify --template-package PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--require-case-runs] [--require-workflow-runs] [--json] [--force]
  agent-testbench template-package import --from PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--audit] [--require-audit-ok] [--force]
  agent-testbench profile init --output PATH [--id ID] [--display-name NAME] [--force]
  agent-testbench profile install --from PATH [--profile-home PATH] [--force]
  agent-testbench profile pack --profile PATH_OR_ID --output PATH [--profile-home PATH] [--force]
  agent-testbench profile list [--profile-home PATH] [--json]
  agent-testbench profile inspect --profile PATH_OR_ID [--profile-home PATH]
  agent-testbench profile export --store NAME_OR_DSN --output PATH [--force] [--json]
  agent-testbench profile audit --profile PATH_OR_ID --offline-template-package [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--force]
  agent-testbench profile audit-plan --profile PATH_OR_ID --offline-template-package [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--force]
  agent-testbench profile doctor --profile PATH_OR_ID --case-id ID [--profile-home PATH] [--json]
  agent-testbench profile repair --from-manifest PATH [--profile PATH_OR_ID] [--profile-home PATH] [--apply] [--json]
  agent-testbench profile generation-plan openapi --from PATH [--service-id ID] [--evidence-dir PATH] [--output-dir PATH] [--json]
  agent-testbench profile import-plan openapi --from PATH [--service-id ID] [--evidence-dir PATH] [--output-dir PATH] [--json]
  agent-testbench profile import-plan http-capture --from PATH [--service-id ID] [--evidence-dir PATH] [--output-dir PATH] [--json]
  agent-testbench profile verify --profile PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--require-case-runs] [--require-workflow-runs] [--json] [--force]
  agent-testbench profile import --from PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--audit] [--require-audit-ok] [--force]
  agent-testbench config publish --from PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--audit] [--require-audit-ok] [--force]
  agent-testbench executor plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--json]
  agent-testbench evidence import --from PATH --profile ID [--store NAME_OR_DSN]
  agent-testbench evidence list [--store NAME_OR_DSN] [--run ID] [--json]
  agent-testbench evidence tasks [--store NAME_OR_DSN] --run ID [--step ID] [--case ID] [--kind KIND] [--status STATUS] [--json]
  agent-testbench trace topology collect --run ID [--store NAME_OR_DSN] --trace-graphql-url URL [--step ID] [--case ID] [--request ID] [--endpoint TEXT] [--trace-id ID] [--json]
  agent-testbench replay evidence --trace-id ID [--json]
  agent-testbench workflow discover [--store NAME_OR_DSN] [--filter TEXT] [--json]
  agent-testbench workflow discover --profile PATH_OR_ID --offline-template-package [--profile-home PATH] [--filter TEXT] [--json]
  agent-testbench workflow plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] --workflow ID [--json]
  agent-testbench workflow audit --workflow ID [--store NAME_OR_DSN] [--json]
  agent-testbench workflow audit --profile PATH --offline-template-package --workflow ID [--store NAME_OR_DSN] [--json]
  agent-testbench workflow runs [--store NAME_OR_DSN] [--json]
  agent-testbench workflow run --run ID [--store NAME_OR_DSN] [--json]
  agent-testbench workflow step --run ID --step ID [--store NAME_OR_DSN] [--json]
  agent-testbench workflow latest-step --workflow ID --step ID [--store NAME_OR_DSN] [--json]
  agent-testbench workflow gate --run ID [--store NAME_OR_DSN] [--require-passed] [--require-steps] [--require-evidence] [--json]
  agent-testbench workflow report --workflow ID [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--base-url URL] [--output-dir PATH] [--json]
  agent-testbench workflow acceptance start --server-url URL --workflow ID --request-id ID [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench workflow acceptance report --server-url URL --run ID [--json]
  agent-testbench baseline get --profile ID --subject ID [--store NAME_OR_DSN]
  agent-testbench baseline set --profile ID --subject ID --status STATUS [--required] [--store NAME_OR_DSN]
  agent-testbench template render [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] --template ID [--fixture ID]
  agent-testbench interface-node discover [--store NAME_OR_DSN] [--filter TEXT] [--json]
  agent-testbench interface-node discover --profile PATH_OR_ID --offline-template-package [--profile-home PATH] [--filter TEXT] [--json]
  agent-testbench interface-node coverage [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--workflow ID] [--json]
  agent-testbench interface-node coverage-gaps [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--workflow ID] [--json]
  agent-testbench interface-node case audit --profile PATH --node ID [--json]
  agent-testbench interface-node case draft --profile PATH --node ID --case-id ID [--title TEXT] [--case-path PATH] [--method METHOD] [--path PATH] [--tag TAG] [--priority PRIORITY] [--owner OWNER] [--output PATH] [--json]
  agent-testbench interface-node case apply --profile PATH --file PATH [--json]
  agent-testbench interface-node case report --node ID [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--base-url URL] [--output-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case discover [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
  agent-testbench case discover --profile PATH_OR_ID --offline-template-package [--profile-home PATH] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
  agent-testbench case suite report [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--base-url URL] [--output-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case suite coverage [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
  agent-testbench case suite stability [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--limit N] [--json]
  agent-testbench case suite priority [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--signal TEXT] [--change TEXT] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--limit N] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case suite brief [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--signal TEXT] [--change TEXT] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--limit N] [--stability-limit N] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case suite quality [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
  agent-testbench case suite quality-plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
  agent-testbench case suite quality-report [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--output-dir PATH] [--json]
  agent-testbench case suite inspect [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
  agent-testbench case suite plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--action ACTION] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case suite impact [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--signal TEXT] [--change TEXT] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--action ACTION] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case suite impact-report [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--signal TEXT] [--change TEXT] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--action ACTION] [--request-id ID] [--base-url URL] [--output-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case runs [--store NAME_OR_DSN] [--run ID] [--json]
  agent-testbench case evidence [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--json]
  agent-testbench case timing [--store NAME_OR_DSN] [--kind KIND] [--max-age-minutes N] [--json]
  agent-testbench case batch start --server-url URL [--case ID]... [--node ID]... [--workflow ID] [--suite NAME] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
  agent-testbench case batch report --server-url URL --run ID [--json]
  agent-testbench case run --case PATH [--base-url URL] [--override KEY=VALUE] [--evidence-dir PATH] [--run-id ID] [--dry-run] [--json]
  agent-testbench case run --case-id ID [--base-url URL] [--override KEY=VALUE] [--evidence-dir PATH] [--store NAME_OR_DSN] [--run-id ID] [--json]
  agent-testbench case incomplete-batches [--profile PATH_OR_ID] [--store NAME_OR_DSN] [--json]
  agent-testbench case diagnose [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--json]
  agent-testbench case gate [--store NAME_OR_DSN] [--run ID] [--require-no-failures] [--require-evidence] [--min-passed N] [--json]
  agent-testbench serve [--profile PATH_OR_ID] [--profile-home PATH] [--host HOST] [--port PORT] [--store NAME_OR_DSN]
  agent-testbench help

Serve reads profile catalog data from the local Store. When --profile is set,
the external bundle is first published into the Store/read-model, then served
from that indexed view.`
}

func applyEnvironmentServiceRepoUpdate(item map[string]any, update map[string]string) {
	keyMap := map[string]string{
		"url":      "repo",
		"branch":   "branch",
		"ref":      "ref",
		"checkout": "checkout",
	}
	for repoKey, serviceKey := range keyMap {
		value, ok := update[repoKey]
		if !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			delete(item, serviceKey)
			continue
		}
		item[serviceKey] = value
	}
}

func printPostgresStoreStatus(status postgres.SchemaStatusResult) {
	pending := status.TargetVersion - status.CurrentVersion
	if pending < 0 {
		pending = 0
	}
	fmt.Println("Store: postgres")
	fmt.Printf("URL: %s\n", maskStoreURL(status.URL))
	fmt.Printf("Version: %d\n", status.CurrentVersion)
	fmt.Printf("Target: %d\n", status.TargetVersion)
	fmt.Printf("Pending: %d\n", pending)
}

func printMySQLStoreStatus(status mysql.SchemaStatusResult) {
	pending := status.TargetVersion - status.CurrentVersion
	if pending < 0 {
		pending = 0
	}
	fmt.Println("Store: mysql")
	fmt.Printf("URL: %s\n", maskStoreURL(status.URL))
	fmt.Printf("Version: %d\n", status.CurrentVersion)
	fmt.Printf("Target: %d\n", status.TargetVersion)
	fmt.Printf("Pending: %d\n", pending)
}

func printInterfaceNodeCoverage(payload map[string]any, gapsOnly bool) {
	if gapsOnly {
		fmt.Printf("Interface Node Coverage Gaps: %s\n", valueString(payload["workflowId"]))
		summary := mapFromReportAny(payload["summary"])
		fmt.Printf("Total Steps: %d\n", intFromReportAny(summary["totalSteps"]))
		fmt.Printf("Gaps: %d\n", intFromReportAny(summary["gapCount"]))
		for _, item := range listFromReportAny(payload["gaps"]) {
			row := mapFromReportAny(item)
			fmt.Printf("Gap: %s Node: %s Case: %s\n", valueString(row["stepId"]), valueString(row["nodeId"]), valueString(row["caseId"]))
		}
		return
	}
	fmt.Printf("Interface Node Coverage: %s\n", valueString(payload["workflowId"]))
	summary := mapFromReportAny(payload["summary"])
	fmt.Printf("Total Steps: %d\n", intFromReportAny(summary["totalSteps"]))
	fmt.Printf("Mapped Steps: %d\n", intFromReportAny(summary["mappedSteps"]))
	fmt.Printf("Unmapped Steps: %d\n", intFromReportAny(summary["unmappedSteps"]))
	for _, item := range listFromReportAny(payload["rows"]) {
		row := mapFromReportAny(item)
		fmt.Printf("Step: %s Node: %s Mapped: %t Admission: %s\n", valueString(row["stepId"]), valueString(row["nodeId"]), boolFromReportAny(row["mapped"]), valueString(row["admissionStatus"]))
	}
}

func auditInterfaceNodeCaseExecutionConfigs(bundle profile.Bundle, nodeID string) interfaceNodeCaseAuditReport {
	configs := caseExecutionConfigIDs(bundle.TemplateConfigs)
	report := interfaceNodeCaseAuditReport{ProfileID: bundle.ID, NodeID: nodeID}
	for _, item := range bundle.APICases {
		if item.NodeID != nodeID {
			continue
		}
		report.Counts.Cases++
		if configID := configs[item.ID]; configID != "" {
			report.Counts.Configured++
			report.Configured = append(report.Configured, interfaceNodeCaseConfigured{CaseID: item.ID, ConfigID: configID})
			continue
		}
		report.Counts.Missing++
		report.Missing = append(report.Missing, interfaceNodeCaseMissing{CaseID: item.ID, Title: firstNonEmpty(item.DisplayName, item.ID)})
	}
	report.OK = report.Counts.Cases > 0 && report.Counts.Missing == 0
	return report
}

func printInterfaceNodeCaseAudit(report interfaceNodeCaseAuditReport) {
	fmt.Printf("Profile: %s\n", report.ProfileID)
	fmt.Printf("Interface Node: %s\n", report.NodeID)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Cases: %d\n", report.Counts.Cases)
	fmt.Printf("Configured: %d\n", report.Counts.Configured)
	fmt.Printf("Missing: %d\n", report.Counts.Missing)
	for _, item := range report.Missing {
		fmt.Printf("- missing case execution: %s\n", item.CaseID)
	}
}

func draftInterfaceNodeCase(bundle profile.Bundle, nodeID string, caseID string, title string, casePath string, method string, requestPath string, tags []string, priority string, owner string) (interfaceNodeCaseDraftReport, error) {
	node, ok := findInterfaceNode(bundle.InterfaceNodes, nodeID)
	if !ok {
		return interfaceNodeCaseDraftReport{}, fmt.Errorf("interface node %q not found", nodeID)
	}
	caseID = strings.TrimSpace(caseID)
	if caseExists(bundle.APICases, caseID) {
		return interfaceNodeCaseDraftReport{}, fmt.Errorf("api case %q already exists", caseID)
	}
	method = strings.ToUpper(strings.TrimSpace(firstNonEmpty(method, node.Method, "GET")))
	requestPath = strings.TrimSpace(firstNonEmpty(requestPath, node.Path, "/"))
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}
	title = strings.TrimSpace(firstNonEmpty(title, node.DisplayName, caseID))
	if strings.TrimSpace(casePath) == "" {
		casePath = filepath.ToSlash(filepath.Join("api-cases", safeCaseFileName(caseID)+".json"))
	}
	apiCase := profile.APICase{
		ID:          caseID,
		DisplayName: title,
		Description: "Generated draft for " + firstNonEmpty(node.DisplayName, node.ID) + ".",
		NodeID:      node.ID,
		Tags:        casesuite.NormalizeStringList(tags),
		Priority:    strings.TrimSpace(priority),
		Owner:       strings.TrimSpace(owner),
		Status:      "active",
		SortOrder:   nextCaseSortOrder(bundle.APICases),
		CasePath:    filepath.ToSlash(casePath),
	}
	caseFile := caseFileInput{
		Path: apiCase.CasePath,
		Case: apicase.Case{
			ID:    caseID,
			Title: title,
			Request: apicase.Request{
				Method:  method,
				Path:    requestPath,
				Headers: draftCaseHeaders(method),
				Body:    draftCaseBody(method),
			},
			Assertions: apicase.Assertions{ExpectedStatusCodes: []int{http.StatusOK}},
		},
	}
	configJSON, err := compactJSONValue(map[string]any{
		"caseId": caseID,
		"caseExecution": map[string]any{
			"method":            method,
			"nodeId":            node.ID,
			"path":              requestPath,
			"expectedHttpCodes": []int{http.StatusOK},
		},
	})
	if err != nil {
		return interfaceNodeCaseDraftReport{}, err
	}
	config := profile.TemplateConfig{
		ID:          "cfg." + caseID,
		TemplateID:  "case-execution",
		NodeID:      node.ID,
		ScopeType:   "case",
		ScopeID:     caseID,
		Title:       title + " execution",
		Description: "Generated draft execution config.",
		ConfigJSON:  configJSON,
		Status:      "active",
		SortOrder:   apiCase.SortOrder,
	}
	applyBundle := interfaceNodeCaseApplyRequest{
		APICases:        []profile.APICase{apiCase},
		TemplateConfigs: []templateConfigInput{{TemplateConfig: config}},
		CaseFiles:       []caseFileInput{caseFile},
	}
	return interfaceNodeCaseDraftReport{
		OK:             true,
		ProfileID:      bundle.ID,
		NodeID:         node.ID,
		CaseID:         caseID,
		CasePath:       apiCase.CasePath,
		APICase:        apiCase,
		TemplateConfig: config,
		CaseFile:       caseFile,
		ApplyBundle:    applyBundle,
	}, nil
}

func applyInterfaceNodeCaseConfigs(profilePath string, requestPath string) (interfaceNodeCaseApplyResult, error) {
	raw, err := os.ReadFile(requestPath)
	if err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("read case config bundle %s: %w", requestPath, err)
	}
	var request interfaceNodeCaseApplyRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("decode case config bundle %s: %w", requestPath, err)
	}
	request.APICases = append(request.APICases, request.InterfaceNodeCases...)
	if len(request.TemplateConfigs) == 0 && len(request.APICases) == 0 && len(request.CaseFiles) == 0 {
		return interfaceNodeCaseApplyResult{}, errors.New("case config bundle must include apiCases, templateConfigs, or caseFiles")
	}
	configs := make([]profile.TemplateConfig, 0, len(request.TemplateConfigs))
	for _, item := range request.TemplateConfigs {
		config, err := normalizeTemplateConfigInput(item)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		configs = append(configs, config)
	}
	apiCases := make([]profile.APICase, 0, len(request.APICases))
	for _, item := range request.APICases {
		apiCase, err := normalizeAPICaseInput(item)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		apiCases = append(apiCases, apiCase)
	}
	if err := writeCaseFiles(profilePath, request.CaseFiles); err != nil {
		return interfaceNodeCaseApplyResult{}, err
	}
	catalogPath := filepath.Join(profilePath, "catalog.json")
	payload, existingConfigs, existingCases, err := readCatalogCaseAssets(catalogPath)
	if err != nil {
		return interfaceNodeCaseApplyResult{}, err
	}
	if len(configs) > 0 {
		merged := mergeTemplateConfigs(existingConfigs, configs)
		configRaw, err := json.Marshal(merged)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		payload["templateConfigs"] = configRaw
	}
	if len(apiCases) > 0 {
		merged := mergeProfileAPICases(existingCases, apiCases)
		casesRaw, err := json.Marshal(merged)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		payload["interfaceNodeCases"] = casesRaw
		delete(payload, "apiCases")
	}
	if _, ok := payload["schemaVersion"]; !ok {
		payload["schemaVersion"] = json.RawMessage(`"1"`)
	}
	next, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return interfaceNodeCaseApplyResult{}, err
	}
	next = append(next, '\n')
	if err := os.WriteFile(catalogPath, next, 0o644); err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("write profile catalog %s: %w", catalogPath, err)
	}
	if _, err := profile.Load(profilePath); err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("profile catalog is invalid after apply: %w", err)
	}
	return interfaceNodeCaseApplyResult{Applied: len(configs), Cases: len(apiCases), Files: len(request.CaseFiles)}, nil
}

func buildWorkflowGateReport(ctx context.Context, runtime store.Store, options workflowGateOptions) (workflowGateReport, error) {
	run, err := runtime.GetRun(ctx, strings.TrimSpace(options.RunID))
	if err != nil {
		return workflowGateReport{}, err
	}
	caseRuns, err := runtime.ListAPICaseRuns(ctx, run.ID)
	if err != nil {
		return workflowGateReport{}, err
	}
	evidence, err := runtime.ListEvidence(ctx, run.ID)
	if err != nil {
		return workflowGateReport{}, err
	}
	caseRunIndex := indexWorkflowGateCaseRuns(caseRuns)
	evidenceCountByCaseRun := indexWorkflowGateEvidence(evidence)

	report := workflowGateReport{
		RunID:           run.ID,
		WorkflowID:      run.WorkflowID,
		Status:          run.Status,
		FailedSteps:     []workflowGateStep{},
		MissingEvidence: []workflowGateStep{},
		NextActions:     []string{},
		Warnings:        []string{},
	}
	steps := workflowGateSteps(run.SummaryJSON)
	report.Counts.Steps = len(steps)
	report.Counts.CaseRuns = len(caseRuns)
	for _, rawStep := range steps {
		step := workflowGateStepFrom(rawStep, caseRunIndex.byID, caseRunIndex.byStep, caseRunIndex.byCase, evidenceCountByCaseRun)
		addWorkflowGateStep(&report, step)
	}
	report.Gates = workflowGateGates{
		RunPassed:        strings.EqualFold(run.Status, store.StatusPassed),
		StepsPresent:     report.Counts.Steps > 0,
		StepsPassed:      report.Counts.Steps > 0 && report.Counts.FailedSteps == 0 && report.Counts.OtherSteps == 0,
		EvidenceComplete: report.Counts.Steps > 0 && len(report.MissingEvidence) == 0,
	}
	report.OK = (!options.RequirePassed || report.Gates.RunPassed) &&
		(!options.RequireSteps || (report.Gates.StepsPresent && report.Gates.StepsPassed)) &&
		(!options.RequireEvidence || report.Gates.EvidenceComplete)
	report.NextActions = workflowGateNextActions(report, options)
	return report, nil
}

type workflowGateCaseRunIndex struct {
	byID   map[string]store.APICaseRun
	byCase map[string][]store.APICaseRun
	byStep map[string][]store.APICaseRun
}

func indexWorkflowGateCaseRuns(caseRuns []store.APICaseRun) workflowGateCaseRunIndex {
	index := workflowGateCaseRunIndex{
		byID:   map[string]store.APICaseRun{},
		byCase: map[string][]store.APICaseRun{},
		byStep: map[string][]store.APICaseRun{},
	}
	for _, item := range caseRuns {
		index.byID[item.ID] = item
		index.byCase[item.CaseID] = append(index.byCase[item.CaseID], item)
		if stepID := apiCaseRunStepID(item); stepID != "" {
			index.byStep[stepID] = append(index.byStep[stepID], item)
		}
	}
	return index
}

func indexWorkflowGateEvidence(evidence []store.EvidenceRecord) map[string]int {
	out := map[string]int{}
	for _, record := range evidence {
		if strings.TrimSpace(record.CaseRunID) != "" {
			out[record.CaseRunID]++
		}
	}
	return out
}

func addWorkflowGateStep(report *workflowGateReport, step workflowGateStep) {
	switch {
	case strings.EqualFold(step.Status, store.StatusPassed):
		report.Counts.PassedSteps++
	case strings.EqualFold(step.Status, store.StatusFailed):
		report.Counts.FailedSteps++
		report.FailedSteps = append(report.FailedSteps, step)
	default:
		report.Counts.OtherSteps++
		report.FailedSteps = append(report.FailedSteps, step)
	}
	if step.EvidenceCount > 0 {
		report.Counts.EvidenceComplete++
		return
	}
	report.MissingEvidence = append(report.MissingEvidence, step)
}

func postProcessTaskMatches(row store.PostProcessTask, filter evidenceTaskFilter) bool {
	if filter.StepID != "" && row.StepID != filter.StepID {
		return false
	}
	if filter.CaseID != "" && row.CaseID != filter.CaseID {
		return false
	}
	if filter.Kind != "" && row.Kind != filter.Kind {
		return false
	}
	if filter.Status != "" && row.Status != filter.Status {
		return false
	}
	return true
}

func executeCaseSuiteQualityReport(ctx context.Context, bundle profile.Bundle, sourceStore store.Store, sourceStoreURL string, filters caseListFilter, cases []profile.APICase, outputDir string) (caseSuiteQualityReport, error) {
	started := time.Now()
	plan, err := casesuite.QualityPlan(ctx, bundle, sourceStore, caseSuiteFilter(filters), cases)
	if err != nil {
		return caseSuiteQualityReport{}, err
	}
	report := caseSuiteQualityReport{
		OK:             true,
		ProfileID:      bundle.ID,
		Title:          "Case Suite Quality Report",
		ElapsedMs:      time.Since(started).Milliseconds(),
		GeneratedAt:    time.Now().UTC(),
		Filters:        normalizeCaseListFilter(filters),
		Counts:         plan.Counts,
		QualityPlan:    plan,
		Warnings:       append([]string(nil), plan.Warnings...),
		SourceStoreURL: sourceStoreURL,
	}
	if sourceStore == nil {
		report.Warnings = append(report.Warnings, "source Store was not available; report used profile bundle only")
	}
	if err := writeCaseSuiteQualityReportFiles(outputDir, &report); err != nil {
		return caseSuiteQualityReport{}, err
	}
	return report, nil
}
