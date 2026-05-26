package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPublishCommandIndexesBundleInStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeEmptyProfileBundle(t)

	out := runCLI(t, "config", "publish", "--from", profileDir, "--store", "sqlite://"+dbPath, "--json")

	var report struct {
		ProfileID     string   `json:"profileId"`
		BundleDigest  string   `json:"bundleDigest"`
		ReadModels    []string `json:"readModels"`
		ConfigVersion struct {
			ID           string `json:"id"`
			ProfileID    string `json:"profileId"`
			BundleDigest string `json:"bundleDigest"`
			Active       bool   `json:"active"`
		} `json:"configVersion"`
		CatalogIndex struct {
			ProfileID string `json:"profileId"`
		} `json:"catalogIndex"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode config publish report: %v\n%s", err, out)
	}
	if report.ProfileID != "empty" || report.CatalogIndex.ProfileID != "empty" || !strings.HasPrefix(report.BundleDigest, "sha256:") {
		t.Fatalf("config publish report = %#v", report)
	}
	if report.ConfigVersion.ID == "" || report.ConfigVersion.ProfileID != "empty" || report.ConfigVersion.BundleDigest != report.BundleDigest || !report.ConfigVersion.Active {
		t.Fatalf("config version = %#v", report.ConfigVersion)
	}
	if strings.Join(report.ReadModels, ",") != "interface-nodes,catalog,dashboard" {
		t.Fatalf("config publish read models = %#v", report.ReadModels)
	}
	if got := sqliteScalar(t, dbPath, "select value from kv where key = 'active_profile_id';"); got != "empty" {
		t.Fatalf("active config profile = %q", got)
	}
	if got := sqliteScalar(t, dbPath, "select bundle_digest from config_versions where active = 1;"); got != report.BundleDigest {
		t.Fatalf("active config digest = %q, want %q", got, report.BundleDigest)
	}
	if got := sqliteScalar(t, dbPath, "select config_version_id from config_read_model where profile_id = 'empty' and model_key = 'interface-nodes';"); got != report.ConfigVersion.ID {
		t.Fatalf("interface nodes read model version = %q, want %q", got, report.ConfigVersion.ID)
	}
	if got := sqliteScalar(t, dbPath, "select config_version_id from config_read_model where profile_id = 'empty' and model_key = 'catalog';"); got != report.ConfigVersion.ID {
		t.Fatalf("catalog read model version = %q, want %q", got, report.ConfigVersion.ID)
	}
}

func TestConfigPublishCommandMaterializesInterfaceNodeDetailReadModels(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeInterfaceNodeCaseProfile(t)

	out := runCLI(t, "config", "publish", "--from", profileDir, "--store", "sqlite://"+dbPath, "--json")

	var report struct {
		ConfigVersion struct {
			ID string `json:"id"`
		} `json:"configVersion"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode config publish report: %v\n%s", err, out)
	}
	if got := sqliteScalar(t, dbPath, "select config_version_id from config_read_model where profile_id = 'sample' and model_key = 'interface-node:node.alpha';"); got != report.ConfigVersion.ID {
		t.Fatalf("interface node detail read model version = %q, want %q", got, report.ConfigVersion.ID)
	}
	if got := sqliteScalar(t, dbPath, "select json_extract(payload_json, '$.source.kind') from config_read_model where profile_id = 'sample' and model_key = 'interface-node:node.alpha';"); got != "read-model" {
		t.Fatalf("interface node detail source kind = %q", got)
	}
}

func TestConfigPublishCommandMaterializesInterfaceNodeCoverageReadModels(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	profileDir := writeInterfaceNodeCoverageProfile(t)

	out := runCLI(t, "config", "publish", "--from", profileDir, "--store", "sqlite://"+dbPath, "--json")

	var report struct {
		ConfigVersion struct {
			ID string `json:"id"`
		} `json:"configVersion"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode config publish report: %v\n%s", err, out)
	}
	if got := sqliteScalar(t, dbPath, "select config_version_id from config_read_model where profile_id = 'sample' and model_key = 'interface-node-coverage:workflow.alpha';"); got != report.ConfigVersion.ID {
		t.Fatalf("interface node coverage read model version = %q, want %q", got, report.ConfigVersion.ID)
	}
	if got := sqliteScalar(t, dbPath, "select json_extract(payload_json, '$.source.kind') from config_read_model where profile_id = 'sample' and model_key = 'interface-node-coverage-gaps:workflow.alpha';"); got != "read-model" {
		t.Fatalf("interface node coverage gaps source kind = %q", got)
	}
}
