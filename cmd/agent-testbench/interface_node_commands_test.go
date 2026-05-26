package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
)

func TestInterfaceNodeCoverageCommandCanEmitJSON(t *testing.T) {
	fixture := writeUniqueInterfaceNodeCoverageProfile(t)
	configureNamedPostgreSQLActiveStore(t, "daily-interface-coverage-pg")
	runInterfaceNodeCoverageCommandCanEmitJSON(t, fixture, "PostgreSQL")
}

func TestInterfaceNodeCoverageCommandUsesNamedMySQLActiveStore(t *testing.T) {
	fixture := writeUniqueInterfaceNodeCoverageProfile(t)
	configureNamedMySQLActiveStore(t, "daily-interface-coverage-mysql")
	runInterfaceNodeCoverageCommandCanEmitJSON(t, fixture, "MySQL")
}

func runInterfaceNodeCoverageCommandCanEmitJSON(t *testing.T, fixture interfaceNodeCoverageFixture, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", fixture.dir)

	out := runCLI(t, "interface-node", "coverage", "--workflow", fixture.workflowID, "--json")

	var report struct {
		OK      bool `json:"ok"`
		Summary struct {
			TotalSteps  int `json:"totalSteps"`
			MappedSteps int `json:"mappedSteps"`
		} `json:"summary"`
		Rows []struct {
			WorkflowID string `json:"workflowId"`
			StepID     string `json:"stepId"`
			NodeID     string `json:"nodeId"`
			Mapped     bool   `json:"mapped"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s interface-node coverage json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Summary.TotalSteps != 1 || report.Summary.MappedSteps != 1 {
		t.Fatalf("%s coverage summary = %#v", label, report.Summary)
	}
	if len(report.Rows) != 1 || report.Rows[0].WorkflowID != fixture.workflowID || report.Rows[0].StepID != fixture.stepID || report.Rows[0].NodeID != fixture.nodeID || !report.Rows[0].Mapped {
		t.Fatalf("%s coverage rows = %#v", label, report.Rows)
	}
}

func TestInterfaceNodeCoverageGapsCommandCanEmitJSON(t *testing.T) {
	fixture := writeUniqueInterfaceNodeCoverageGapsProfile(t)
	configureNamedPostgreSQLActiveStore(t, "daily-interface-coverage-gaps-pg")
	runInterfaceNodeCoverageGapsCommandCanEmitJSON(t, fixture, "PostgreSQL")
}

func TestInterfaceNodeCoverageGapsCommandUsesNamedMySQLActiveStore(t *testing.T) {
	fixture := writeUniqueInterfaceNodeCoverageGapsProfile(t)
	configureNamedMySQLActiveStore(t, "daily-interface-coverage-gaps-mysql")
	runInterfaceNodeCoverageGapsCommandCanEmitJSON(t, fixture, "MySQL")
}

func runInterfaceNodeCoverageGapsCommandCanEmitJSON(t *testing.T, fixture interfaceNodeCoverageFixture, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", fixture.dir)

	out := runCLI(t, "interface-node", "coverage-gaps", "--workflow", fixture.workflowID, "--json")

	var report struct {
		OK      bool `json:"ok"`
		Summary struct {
			TotalSteps int `json:"totalSteps"`
			GapCount   int `json:"gapCount"`
		} `json:"summary"`
		Gaps []struct {
			StepID string `json:"stepId"`
			NodeID string `json:"nodeId"`
			Mapped bool   `json:"mapped"`
		} `json:"gaps"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s interface-node coverage gaps json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Summary.TotalSteps != 1 || report.Summary.GapCount != 1 {
		t.Fatalf("%s coverage gaps summary = %#v", label, report.Summary)
	}
	if len(report.Gaps) != 1 || report.Gaps[0].StepID != fixture.stepID || report.Gaps[0].NodeID != fixture.nodeID || report.Gaps[0].Mapped {
		t.Fatalf("%s coverage gaps = %#v", label, report.Gaps)
	}
}

type interfaceNodeCoverageFixture struct {
	dir        string
	profileID  string
	workflowID string
	serviceID  string
	nodeID     string
	caseID     string
	stepID     string
}

func writeUniqueInterfaceNodeCoverageProfile(t *testing.T) interfaceNodeCoverageFixture {
	t.Helper()
	fixture := newInterfaceNodeCoverageFixture(t)
	writeFile(t, filepath.Join(fixture.dir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [{"id":%q,"displayName":"Service Alpha"}],
  "workflows": [{"id":%q,"displayName":"Workflow Alpha"}],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha","serviceId":%q}],
  "apiCases": [{"id":%q,"displayName":"Case Alpha","nodeId":%q}],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [{"workflowId":%q,"stepId":%q,"nodeId":%q,"caseId":%q,"required":true}],
  "fixtures": []
}`, fixture.profileID, fixture.serviceID, fixture.workflowID, fixture.nodeID, fixture.serviceID, fixture.caseID, fixture.nodeID, fixture.workflowID, fixture.stepID, fixture.nodeID, fixture.caseID))
	return fixture
}

func writeUniqueInterfaceNodeCoverageGapsProfile(t *testing.T) interfaceNodeCoverageFixture {
	t.Helper()
	fixture := newInterfaceNodeCoverageFixture(t)
	writeFile(t, filepath.Join(fixture.dir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [{"id":%q,"displayName":"Workflow Alpha"}],
  "interfaceNodes": [],
  "apiCases": [],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [{"workflowId":%q,"stepId":%q,"nodeId":%q,"caseId":%q,"required":true}],
  "fixtures": []
}`, fixture.profileID, fixture.workflowID, fixture.workflowID, fixture.stepID, fixture.nodeID, fixture.caseID))
	return fixture
}

func newInterfaceNodeCoverageFixture(t *testing.T) interfaceNodeCoverageFixture {
	t.Helper()
	return interfaceNodeCoverageFixture{
		dir:        t.TempDir(),
		profileID:  uniqueTestID(t, "profile.interface-coverage"),
		workflowID: uniqueTestID(t, "workflow.interface-coverage"),
		serviceID:  uniqueTestID(t, "service.interface-coverage"),
		nodeID:     uniqueTestID(t, "node.interface-coverage"),
		caseID:     uniqueTestID(t, "case.interface-coverage"),
		stepID:     uniqueTestID(t, "step.interface-coverage"),
	}
}

func TestInterfaceNodeCaseAuditReportsMissingExecutionConfigs(t *testing.T) {
	dir := writeInterfaceNodeCaseProfile(t)

	out := runCLI(t, "interface-node", "case", "audit", "--profile", dir, "--node", "node.alpha", "--json")

	var report struct {
		OK     bool   `json:"ok"`
		NodeID string `json:"nodeId"`
		Counts struct {
			Cases      int `json:"cases"`
			Configured int `json:"configured"`
			Missing    int `json:"missing"`
		} `json:"counts"`
		Missing []struct {
			CaseID string `json:"caseId"`
		} `json:"missing"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode interface node case audit json: %v\n%s", err, out)
	}
	if report.OK || report.NodeID != "node.alpha" || report.Counts.Cases != 2 || report.Counts.Configured != 1 || report.Counts.Missing != 1 {
		t.Fatalf("audit report = %#v", report)
	}
	if len(report.Missing) != 1 || report.Missing[0].CaseID != "case.beta" {
		t.Fatalf("missing cases = %#v", report.Missing)
	}
}

func TestInterfaceNodeCaseApplyMergesExecutionConfigsIntoProfileCatalog(t *testing.T) {
	dir := writeInterfaceNodeCaseProfile(t)
	requestPath := filepath.Join(t.TempDir(), "case-config.json")
	writeFile(t, requestPath, `{
  "templateConfigs": [
    {
      "id": "cfg.case.beta",
      "templateId": "case-execution",
      "nodeId": "node.alpha",
      "scopeType": "case",
      "scopeId": "case.beta",
      "title": "Case Beta execution",
      "status": "active",
      "sortOrder": 2,
      "config": {
        "caseId": "case.beta",
        "caseExecution": {
          "method": "GET",
          "nodeId": "service.alpha",
          "path": "/beta",
          "expectedHttpCodes": [200]
        }
      }
    }
  ]
}`)

	out := runCLI(t, "interface-node", "case", "apply", "--profile", dir, "--file", requestPath, "--json")

	var result struct {
		Applied int    `json:"applied"`
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode interface node case apply json: %v\n%s", err, out)
	}
	if result.Applied != 1 || result.Profile != dir {
		t.Fatalf("apply result = %#v", result)
	}
	audit := runCLI(t, "interface-node", "case", "audit", "--profile", dir, "--node", "node.alpha", "--json")
	var auditReport struct {
		OK     bool `json:"ok"`
		Counts struct {
			Missing int `json:"missing"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(audit), &auditReport); err != nil {
		t.Fatalf("decode audit after apply: %v\n%s", err, audit)
	}
	if !auditReport.OK || auditReport.Counts.Missing != 0 {
		t.Fatalf("audit after apply = %s", audit)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		TemplateConfigs []struct {
			ConfigJSON string `json:"configJson"`
		} `json:"templateConfigs"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode catalog after apply: %v\n%s", err, raw)
	}
	hasBeta := false
	for _, item := range catalog.TemplateConfigs {
		var config struct {
			CaseID string `json:"caseId"`
		}
		if err := json.Unmarshal([]byte(item.ConfigJSON), &config); err != nil {
			t.Fatalf("decode template config after apply: %v\n%s", err, item.ConfigJSON)
		}
		hasBeta = hasBeta || config.CaseID == "case.beta"
	}
	if !hasBeta || strings.Contains(string(raw), "store.sqlite") {
		t.Fatalf("catalog after apply = %s", raw)
	}
}

func TestInterfaceNodeCaseDraftAndApplyCreatesRunnableMaintainedCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Node Alpha","method":"POST","path":"/v1/items","sortOrder":7}],
  "apiCases": [],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	bundlePath := filepath.Join(t.TempDir(), "case-draft.json")

	out := runCLI(t,
		"interface-node", "case", "draft",
		"--profile", dir,
		"--node", "node.alpha",
		"--case-id", "case.generated",
		"--title", "Generated Case",
		"--tag", "regression",
		"--tag", "smoke",
		"--priority", "p1",
		"--owner", "team-a",
		"--output", bundlePath,
		"--json",
	)
	var draft struct {
		OK             bool   `json:"ok"`
		CaseID         string `json:"caseId"`
		NodeID         string `json:"nodeId"`
		BundlePath     string `json:"bundlePath"`
		CasePath       string `json:"casePath"`
		TemplateConfig struct {
			ConfigJSON string `json:"configJson"`
		} `json:"templateConfig"`
		CaseFile struct {
			Path string       `json:"path"`
			Case apicase.Case `json:"case"`
		} `json:"caseFile"`
	}
	if err := json.Unmarshal([]byte(out), &draft); err != nil {
		t.Fatalf("decode case draft json: %v\n%s", err, out)
	}
	if !draft.OK || draft.CaseID != "case.generated" || draft.NodeID != "node.alpha" || draft.BundlePath != bundlePath || draft.CasePath != "api-cases/case.generated.json" {
		t.Fatalf("case draft = %#v", draft)
	}
	if draft.CaseFile.Path != draft.CasePath || draft.CaseFile.Case.Request.Method != "POST" || draft.CaseFile.Case.Request.Path != "/v1/items" {
		t.Fatalf("case draft file = %#v", draft.CaseFile)
	}
	if !strings.Contains(draft.TemplateConfig.ConfigJSON, `"caseId":"case.generated"`) || !strings.Contains(draft.TemplateConfig.ConfigJSON, `"expectedHttpCodes":[200]`) {
		t.Fatalf("case draft config json = %s", draft.TemplateConfig.ConfigJSON)
	}
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("draft bundle missing: %v", err)
	}

	applyOut := runCLI(t, "interface-node", "case", "apply", "--profile", dir, "--file", bundlePath, "--json")
	var applied struct {
		Applied int `json:"applied"`
		Cases   int `json:"cases"`
		Files   int `json:"files"`
	}
	if err := json.Unmarshal([]byte(applyOut), &applied); err != nil {
		t.Fatalf("decode apply draft json: %v\n%s", err, applyOut)
	}
	if applied.Applied != 1 || applied.Cases != 1 || applied.Files != 1 {
		t.Fatalf("apply draft result = %#v", applied)
	}
	if _, err := os.Stat(filepath.Join(dir, "api-cases", "case.generated.json")); err != nil {
		t.Fatalf("applied runnable case file missing: %v", err)
	}
	loaded, err := profile.Load(dir)
	if err != nil {
		t.Fatalf("load applied profile: %v", err)
	}
	if len(loaded.APICases) != 1 || loaded.APICases[0].ID != "case.generated" || loaded.APICases[0].CasePath != "api-cases/case.generated.json" || loaded.APICases[0].Owner != "team-a" {
		t.Fatalf("loaded applied cases = %#v", loaded.APICases)
	}
	audit := runCLI(t, "interface-node", "case", "audit", "--profile", dir, "--node", "node.alpha", "--json")
	var auditReport struct {
		OK     bool `json:"ok"`
		Counts struct {
			Cases      int `json:"cases"`
			Configured int `json:"configured"`
			Missing    int `json:"missing"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(audit), &auditReport); err != nil {
		t.Fatalf("decode audit after draft apply: %v\n%s", err, audit)
	}
	if !auditReport.OK || auditReport.Counts.Cases != 1 || auditReport.Counts.Configured != 1 || auditReport.Counts.Missing != 0 {
		t.Fatalf("audit after draft apply = %#v", auditReport)
	}
}
