package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jsonID(raw json.RawMessage) string {
	var payload struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.ID)
}

func writeAPICaseFile(t *testing.T, path string) {
	t.Helper()
	raw := []byte(`{
  "id": "case.alpha",
  "title": "Create Item",
  "request": {
    "method": "POST",
    "path": "/v1/items",
    "headers": {"Content-Type": "application/json"},
    "body": {"id": "item-001"}
  },
  "assertions": {
    "expectedStatusCodes": [200],
    "responseContains": ["created"]
  }
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write api case: %v", err)
	}
}

func writeEmptyProfileBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "empty",
  "displayName": "Empty Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [],
  "apiCases": [],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	return dir
}

func writeWorkflowProfile(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [],
  "apiCases": [],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	writeFile(t, filepath.Join(dir, "workflows", "workflow.json"), `{"id":"workflow.alpha","displayName":"Workflow Alpha"}`)
	writeFile(t, filepath.Join(dir, "interface-nodes", "node.json"), `{"id":"node.alpha","displayName":"Node Alpha"}`)
	writeFile(t, filepath.Join(dir, "cases", "case.json"), `{"id":"case.alpha","displayName":"Case Alpha","nodeId":"node.alpha"}`)
	writeFile(t, filepath.Join(dir, "workflow-bindings", "binding.json"), `{"workflowId":"workflow.alpha","stepId":"step.one","nodeId":"node.alpha","caseId":"case.alpha","required":true}`)
}

func writeTemplateProfile(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [],
  "apiCases": [],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	writeFile(t, filepath.Join(dir, "request-templates", "template.json"), `{
  "id": "template.create",
  "method": "POST",
  "path": "/v1/items/{{.itemId}}",
  "templateJson": "{\"id\":\"{{.itemId}}\",\"quantity\":{{.quantity}}}"
}`)
	writeFile(t, filepath.Join(dir, "fixtures", "fixture.json"), `{
  "id": "fixture.item",
  "kind": "json",
  "dataJson": "{\"itemId\":\"item-001\",\"quantity\":3}"
}`)
}

func writeInterfaceNodeCaseProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Node Alpha"}],
  "apiCases": [
    {"id":"case.alpha","displayName":"Case Alpha","nodeId":"node.alpha"},
    {"id":"case.beta","displayName":"Case Beta","nodeId":"node.alpha"}
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
      "id": "cfg.case.alpha",
      "templateId": "case-execution",
      "nodeId": "node.alpha",
      "scopeType": "case",
      "scopeId": "case.alpha",
      "title": "Case Alpha execution",
      "status": "active",
      "sortOrder": 1,
      "configJson": "{\"caseId\":\"case.alpha\",\"caseExecution\":{\"method\":\"GET\",\"nodeId\":\"service.alpha\",\"path\":\"/alpha\",\"expectedHttpCodes\":[200]}}"
    }
  ]
}`)
	return dir
}

func writeProfileWithCatalogCases(t *testing.T, caseIDs []string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Node Alpha"}],
  "apiCases": [],
  "requestTemplates": [{"id":"tpl.alpha","nodeId":"node.alpha","method":"POST","path":"/alpha","templateJson":"{\"id\":\"{{serial:CASE}}\"}"}],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	var cases []map[string]any
	for index, id := range caseIDs {
		cases = append(cases, map[string]any{
			"id":                id,
			"nodeId":            "node.alpha",
			"title":             "Case " + id,
			"requestTemplateId": "tpl.alpha",
			"expectedJson":      `{"expectedHttpCodes":[200]}`,
			"status":            "active",
			"sortOrder":         index + 1,
		})
		writeFile(t, filepath.Join(dir, "cases", id+".json"), `{"id":"`+id+`","nodeId":"node.alpha"}`)
	}
	rawCases, err := json.MarshalIndent(map[string]any{"interfaceNodeCases": cases}, "", "  ")
	if err != nil {
		t.Fatalf("marshal catalog cases: %v", err)
	}
	writeFile(t, filepath.Join(dir, "catalog.json"), string(rawCases))
	return dir
}

func writeProfileRepairManifest(t *testing.T, profileDir string, caseIDs []string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(profileDir, "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog struct {
		InterfaceNodeCases []json.RawMessage `json:"interfaceNodeCases"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	want := map[string]bool{}
	for _, id := range caseIDs {
		want[id] = true
	}
	var selected []json.RawMessage
	caseFiles := map[string]string{}
	for _, item := range catalog.InterfaceNodeCases {
		if !want[jsonID(item)] {
			continue
		}
		selected = append(selected, item)
		casePath := filepath.Join(profileDir, "cases", jsonID(item)+".json")
		content, err := os.ReadFile(casePath)
		if err != nil {
			t.Fatalf("read case file: %v", err)
		}
		caseFiles[casePath] = string(content)
	}
	manifest := map[string]any{
		"profilePath":  profileDir,
		"catalogPath":  filepath.Join(profileDir, "catalog.json"),
		"caseIds":      caseIDs,
		"catalogCases": selected,
		"caseFiles":    caseFiles,
	}
	rawManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "repair-manifest.json")
	writeFile(t, path, string(rawManifest))
	return path
}

func removeProfileCatalogCase(t *testing.T, profileDir string, caseID string) {
	t.Helper()
	path := filepath.Join(profileDir, "catalog.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	var kept []any
	for _, item := range catalog["interfaceNodeCases"].([]any) {
		rawItem, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal case: %v", err)
		}
		if jsonID(rawItem) != caseID {
			kept = append(kept, item)
		}
	}
	catalog["interfaceNodeCases"] = kept
	out, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	writeFile(t, path, string(out))
}

func writeWorkflowBatchReportProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [{"id":"workflow.alpha","displayName":"Workflow Alpha","baseStepTimeoutMs":1000}],
  "interfaceNodes": [
    {"id":"node.first","displayName":"First Node","serviceId":"service.alpha","method":"GET","path":"/first"},
    {"id":"node.second","displayName":"Second Node","serviceId":"service.alpha","method":"GET","path":"/second"}
  ],
  "apiCases": [
    {"id":"case.first","displayName":"First Step Case","nodeId":"node.first","sortOrder":1},
    {"id":"case.second","displayName":"Second Step Case","nodeId":"node.second","sortOrder":2}
  ],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [
    {"workflowId":"workflow.alpha","stepId":"first","nodeId":"node.first","caseId":"case.first","required":true,"sortOrder":1},
    {"workflowId":"workflow.alpha","stepId":"second","nodeId":"node.second","caseId":"case.second","required":true,"sortOrder":2}
  ],
  "fixtures": []
}`)
	writeFile(t, filepath.Join(dir, "catalog.json"), `{
  "schemaVersion": "1",
  "templateConfigs": [
    {
      "id": "cfg.step.first",
      "templateId": "case-execution",
      "workflowId": "workflow.alpha",
      "nodeId": "service.alpha",
      "scopeType": "step",
      "scopeId": "first",
      "title": "First Step",
      "status": "active",
      "sortOrder": 1,
      "configJson": "{\"caseId\":\"case.first\",\"caseExecution\":{\"method\":\"GET\",\"nodeId\":\"service.alpha\",\"path\":\"/first\",\"expectedHttpCodes\":[200]},\"exports\":[{\"name\":\"item_id\",\"from\":\"responseBody\",\"path\":\"item_id\"}]}"
    },
    {
      "id": "cfg.step.second",
      "templateId": "case-execution",
      "workflowId": "workflow.alpha",
      "nodeId": "service.alpha",
      "scopeType": "step",
      "scopeId": "second",
      "title": "Second Step",
      "status": "active",
      "sortOrder": 2,
      "configJson": "{\"caseId\":\"case.second\",\"caseExecution\":{\"method\":\"GET\",\"nodeId\":\"service.alpha\",\"path\":\"/second\",\"expectedHttpCodes\":[200]},\"inputs\":[{\"name\":\"item_id\",\"source\":\"previous\"}]}"
    }
  ]
}`)
	return dir
}

type workflowBatchReportFixture struct {
	profileDir     string
	profileID      string
	workflowID     string
	workflowName   string
	nodeFirstID    string
	nodeSecondID   string
	caseFirstID    string
	caseSecondID   string
	firstConfigID  string
	secondConfigID string
}

func writeUniqueWorkflowBatchReportProfile(t *testing.T) workflowBatchReportFixture {
	t.Helper()
	fixture := workflowBatchReportFixture{
		profileDir:     t.TempDir(),
		profileID:      uniqueTestID(t, "profile.workflow-batch-report"),
		workflowID:     uniqueTestID(t, "workflow.alpha"),
		workflowName:   "Workflow Alpha " + strings.ReplaceAll(t.Name(), "/", "-"),
		nodeFirstID:    uniqueTestID(t, "node.first"),
		nodeSecondID:   uniqueTestID(t, "node.second"),
		caseFirstID:    uniqueTestID(t, "case.first"),
		caseSecondID:   uniqueTestID(t, "case.second"),
		firstConfigID:  uniqueTestID(t, "cfg.step.first"),
		secondConfigID: uniqueTestID(t, "cfg.step.second"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [{"id":%q,"displayName":%q,"baseStepTimeoutMs":1000}],
  "interfaceNodes": [
    {"id":%q,"displayName":"First Node","serviceId":"service.alpha","method":"GET","path":"/first"},
    {"id":%q,"displayName":"Second Node","serviceId":"service.alpha","method":"GET","path":"/second"}
  ],
  "apiCases": [
    {"id":%q,"displayName":"First Step Case","nodeId":%q,"sortOrder":1},
    {"id":%q,"displayName":"Second Step Case","nodeId":%q,"sortOrder":2}
  ],
  "requestTemplates": [],
  "templateConfigs": [
    {
      "id": %q,
      "templateId": "case-execution",
      "workflowId": %q,
      "nodeId": "service.alpha",
      "scopeType": "step",
      "scopeId": "first",
      "title": "First Step",
      "status": "active",
      "sortOrder": 1,
      "configJson": %q
    },
    {
      "id": %q,
      "templateId": "case-execution",
      "workflowId": %q,
      "nodeId": "service.alpha",
      "scopeType": "step",
      "scopeId": "second",
      "title": "Second Step",
      "status": "active",
      "sortOrder": 2,
      "configJson": %q
    }
  ],
  "caseDependencies": [],
  "workflowBindings": [
    {"workflowId":%q,"stepId":"first","nodeId":%q,"caseId":%q,"required":true,"sortOrder":1},
    {"workflowId":%q,"stepId":"second","nodeId":%q,"caseId":%q,"required":true,"sortOrder":2}
  ],
  "fixtures": []
}`, fixture.profileID, fixture.workflowID, fixture.workflowName, fixture.nodeFirstID, fixture.nodeSecondID, fixture.caseFirstID, fixture.nodeFirstID, fixture.caseSecondID, fixture.nodeSecondID, fixture.firstConfigID, fixture.workflowID, fmt.Sprintf(`{"caseId":%q,"caseExecution":{"method":"GET","nodeId":"service.alpha","path":"/first","expectedHttpCodes":[200]},"exports":[{"name":"item_id","from":"responseBody","path":"item_id"}]}`, fixture.caseFirstID), fixture.secondConfigID, fixture.workflowID, fmt.Sprintf(`{"caseId":%q,"caseExecution":{"method":"GET","nodeId":"service.alpha","path":"/second","expectedHttpCodes":[200]},"inputs":[{"name":"item_id","source":"previous"}]}`, fixture.caseSecondID), fixture.workflowID, fixture.nodeFirstID, fixture.caseFirstID, fixture.workflowID, fixture.nodeSecondID, fixture.caseSecondID))
	return fixture
}
