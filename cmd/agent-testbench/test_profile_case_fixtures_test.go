package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func writeInterfaceNodeCoverageProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [{"id":"workflow.alpha","displayName":"Workflow Alpha"}],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Node Alpha","serviceId":"service.alpha"}],
  "apiCases": [{"id":"case.alpha","displayName":"Case Alpha","nodeId":"node.alpha"}],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [{"workflowId":"workflow.alpha","stepId":"step.alpha","nodeId":"node.alpha","caseId":"case.alpha","required":true}],
  "fixtures": []
}`)
	return dir
}

func writeInterfaceNodeBatchReportProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Result Lookup","serviceId":"service.alpha","operation":"Result Lookup","method":"GET","path":"/lookup"}],
  "apiCases": [
    {"id":"case.alpha.default","displayName":"Case Alpha Default","nodeId":"node.alpha","payloadTemplateJson":"{\"mode\":\"ok\"}","expectedJson":"{\"expectedHttpCodes\":[200]}","sortOrder":1,"tags":["smoke","regression"],"priority":"p0","owner":"team-a","description":"Default maintained smoke case."},
    {"id":"case.alpha.variant","displayName":"Case Alpha Variant","nodeId":"node.alpha","payloadTemplateJson":"{\"mode\":\"bad\"}","expectedJson":"{\"expectedHttpCodes\":[400]}","sortOrder":2,"tags":["negative"],"priority":"p1","owner":"team-b","description":"Negative maintained variant."}
  ],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	writeFile(t, filepath.Join(dir, "catalog.json"), `{
  "schemaVersion": "1",
  "templateConfigs": [
    {
      "id": "cfg.case.alpha.default",
      "templateId": "case-execution",
      "nodeId": "node.alpha",
      "scopeType": "case",
      "scopeId": "case.alpha.default",
      "title": "Case Alpha Default execution",
      "status": "active",
      "sortOrder": 1,
      "configJson": "{\"caseId\":\"case.alpha.default\",\"caseExecution\":{\"method\":\"GET\",\"nodeId\":\"service.alpha\",\"path\":\"/lookup\",\"query\":{\"mode\":\"ok\"},\"expectedHttpCodes\":[200]}}"
    }
  ]
}`)
	return dir
}

type interfaceNodeBatchReportFixture struct {
	profileDir      string
	profileID       string
	nodeAlphaID     string
	defaultCaseID   string
	variantCaseID   string
	defaultConfigID string
}

func writeUniqueInterfaceNodeBatchReportProfile(t *testing.T) interfaceNodeBatchReportFixture {
	t.Helper()
	fixture := interfaceNodeBatchReportFixture{
		profileDir:      t.TempDir(),
		profileID:       uniqueTestID(t, "profile.interface-node-batch-report"),
		nodeAlphaID:     uniqueTestID(t, "node.alpha"),
		defaultCaseID:   uniqueTestID(t, "case.alpha.default"),
		variantCaseID:   uniqueTestID(t, "case.alpha.variant"),
		defaultConfigID: uniqueTestID(t, "cfg.case.alpha.default"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [],
  "interfaceNodes": [{"id":%q,"displayName":"Result Lookup","serviceId":"service.alpha","operation":"Result Lookup","method":"GET","path":"/lookup"}],
  "apiCases": [
    {"id":%q,"displayName":"Case Alpha Default","nodeId":%q,"payloadTemplateJson":"{\"mode\":\"ok\"}","expectedJson":"{\"expectedHttpCodes\":[200]}","sortOrder":1,"tags":["smoke","regression"],"priority":"p0","owner":"team-a","description":"Default maintained smoke case."},
    {"id":%q,"displayName":"Case Alpha Variant","nodeId":%q,"payloadTemplateJson":"{\"mode\":\"bad\"}","expectedJson":"{\"expectedHttpCodes\":[400]}","sortOrder":2,"tags":["negative"],"priority":"p1","owner":"team-b","description":"Negative maintained variant."}
  ],
  "requestTemplates": [],
  "templateConfigs": [
    {
      "id": %q,
      "templateId": "case-execution",
      "nodeId": %q,
      "scopeType": "case",
      "scopeId": %q,
      "title": "Case Alpha Default execution",
      "status": "active",
      "sortOrder": 1,
      "configJson": %q
    }
  ],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`, fixture.profileID, fixture.nodeAlphaID, fixture.defaultCaseID, fixture.nodeAlphaID, fixture.variantCaseID, fixture.nodeAlphaID, fixture.defaultConfigID, fixture.nodeAlphaID, fixture.defaultCaseID, fmt.Sprintf(`{"caseId":%q,"caseExecution":{"method":"GET","nodeId":%q,"path":"/lookup","query":{"mode":"ok"},"expectedHttpCodes":[200]}}`, fixture.defaultCaseID, fixture.nodeAlphaID)))
	return fixture
}

type caseSuiteQualityFixture struct {
	profileDir           string
	profileID            string
	nodeAlphaID          string
	nodeEmptyID          string
	completeCaseID       string
	gapsCaseID           string
	completeConfigID     string
	suggestedEmptyCaseID string
}

func writeUniqueCaseSuiteQualityProfile(t *testing.T) caseSuiteQualityFixture {
	t.Helper()
	fixture := caseSuiteQualityFixture{
		profileDir:       t.TempDir(),
		profileID:        uniqueTestID(t, "profile.case-suite-quality"),
		nodeAlphaID:      uniqueTestID(t, "node.alpha"),
		nodeEmptyID:      uniqueTestID(t, "node.empty"),
		completeCaseID:   uniqueTestID(t, "case.complete"),
		gapsCaseID:       uniqueTestID(t, "case.gaps"),
		completeConfigID: uniqueTestID(t, "config.case.complete"),
	}
	fixture.suggestedEmptyCaseID = suggestedCaseIDForTest(fixture.nodeEmptyID)
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [],
  "interfaceNodes": [
    {"id":%q,"displayName":"Node Alpha","serviceId":"service.alpha","operation":"Alpha","method":"GET","path":"/alpha"},
    {"id":%q,"displayName":"Node Empty","serviceId":"service.alpha","operation":"Empty","method":"GET","path":"/empty"}
  ],
  "apiCases": [
    {"id":%q,"displayName":"Complete Case","description":"Ready maintained case.","nodeId":%q,"sortOrder":1,"tags":["regression"],"priority":"p0","owner":"team-a","casePath":"cases/complete.json"},
    {"id":%q,"displayName":"Gap Case","nodeId":%q,"sortOrder":2}
  ],
  "requestTemplates": [],
  "templateConfigs": [
    {
      "id": %q,
      "scopeType": "case",
      "scopeId": %q,
      "status": "active",
      "configJson": %q
    }
  ],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`, fixture.profileID, fixture.nodeAlphaID, fixture.nodeEmptyID, fixture.completeCaseID, fixture.nodeAlphaID, fixture.gapsCaseID, fixture.nodeAlphaID, fixture.completeConfigID, fixture.completeCaseID, fmt.Sprintf(`{"caseId":%q,"caseExecution":{"method":"GET","nodeId":%q,"path":"/alpha","expectedHttpCodes":[200]}}`, fixture.completeCaseID, fixture.nodeAlphaID)))
	return fixture
}

func suggestedCaseIDForTest(nodeID string) string {
	value := strings.ToLower(strings.TrimSpace(nodeID))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return "case.case.default"
	}
	return "case." + out + ".default"
}
