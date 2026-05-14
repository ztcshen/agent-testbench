package profile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-test-sandbox/internal/profile"
)

func TestLoadEmptyProfileBundle(t *testing.T) {
	bundle, err := profile.Load(filepath.Join("..", "..", "profiles", "empty"))
	if err != nil {
		t.Fatalf("load empty profile: %v", err)
	}

	if bundle.ID != "empty" || bundle.DisplayName != "Empty Profile" {
		t.Fatalf("bundle identity = %#v", bundle)
	}
	if bundle.Counts().Workflows != 0 || bundle.Counts().APICases != 0 || bundle.Counts().Fixtures != 0 {
		t.Fatalf("empty bundle counts = %#v", bundle.Counts())
	}
}

func TestLoadProfileReturnsActionableValidationErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(`{"displayName":"Missing ID"}`), 0o644); err != nil {
		t.Fatalf("write invalid profile: %v", err)
	}

	_, err := profile.Load(dir)
	if err == nil || !strings.Contains(err.Error(), "profile id is required") {
		t.Fatalf("load invalid profile error = %v", err)
	}
}

func TestLoadProfileReadsSplitAssetDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "split",
  "displayName": "Split Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [],
  "apiCases": [],
  "fixtures": []
}`)
	writeFile(t, filepath.Join(dir, "services", "service.json"), `{"id":"service.one","displayName":"Service One","kind":"http"}`)
	writeFile(t, filepath.Join(dir, "workflows", "workflow.json"), `{"id":"workflow.one","displayName":"Workflow One"}`)
	writeFile(t, filepath.Join(dir, "interface-nodes", "node.json"), `{"id":"node.one","displayName":"Node One","serviceId":"service.one"}`)
	writeFile(t, filepath.Join(dir, "cases", "case.json"), `{"id":"case.one","displayName":"Case One","nodeId":"node.one"}`)
	writeFile(t, filepath.Join(dir, "fixtures", "fixture.json"), `{"id":"fixture.one","displayName":"Fixture One","kind":"json"}`)

	bundle, err := profile.Load(dir)
	if err != nil {
		t.Fatalf("load split profile: %v", err)
	}

	counts := bundle.Counts()
	if counts.Services != 1 || counts.Workflows != 1 || counts.InterfaceNodes != 1 || counts.APICases != 1 || counts.Fixtures != 1 {
		t.Fatalf("split profile counts = %#v", counts)
	}
	if bundle.APICases[0].ID != "case.one" || bundle.InterfaceNodes[0].ServiceID != "service.one" {
		t.Fatalf("split profile assets = %#v", bundle)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
