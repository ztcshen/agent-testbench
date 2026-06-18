package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/profile"
)

const allowTestFixtureProfilePublishEnv = "AGENT_TESTBENCH_ALLOW_TEST_FIXTURE_PROFILE_PUBLISH"

func guardProfilePublishTarget(profilePath string, storeURL string) error {
	backend, err := storeBackendFromURL(storeURL)
	if err != nil {
		return err
	}
	if backend != "mysql" {
		return nil
	}
	bundle, err := profile.Load(profilePath)
	if err != nil {
		return err
	}
	if !isGoTestFixtureProfile(profilePath, bundle.ID) {
		return nil
	}
	database := sqlStoreDatabaseName(storeURL)
	if testFixtureProfilePublishAllowed() && isDedicatedAgentTestBenchStoreDatabaseName(database) {
		return nil
	}
	if database == "" {
		database = "(unknown)"
	}
	return fmt.Errorf("refusing to publish Go test fixture profile %q from %s into %s Store database %q; use a dedicated sandbox/smoke/test/ci database whose name contains agent_testbench, and set %s=1 only for controlled disposable test Store runs",
		bundle.ID, profilePath, backend, database, allowTestFixtureProfilePublishEnv)
}

func isGoTestFixtureProfile(profilePath string, profileID string) bool {
	return profilePathLooksLikeGoTestTempDir(profilePath) || profileIDLooksLikeGoTestFixture(profileID)
}

func profilePathLooksLikeGoTestTempDir(profilePath string) bool {
	cleanPath := filepath.Clean(profilePath)
	tempDir := filepath.Clean(os.TempDir())
	relative, err := filepath.Rel(tempDir, cleanPath)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || relative == ".." {
		return false
	}
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		if strings.HasPrefix(part, "Test") && len(part) > len("Test") {
			return true
		}
	}
	return false
}

func profileIDLooksLikeGoTestFixture(profileID string) bool {
	for _, part := range strings.FieldsFunc(profileID, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	}) {
		part = strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(part, "test") && len(part) > len("test") {
			return true
		}
	}
	return false
}

func testFixtureProfilePublishAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(allowTestFixtureProfilePublishEnv))) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func isDedicatedAgentTestBenchStoreDatabaseName(database string) bool {
	value := strings.ToLower(strings.TrimSpace(database))
	value = strings.ReplaceAll(value, "-", "_")
	return strings.Contains(value, "agent_testbench")
}

func sqlStoreDatabaseName(storeURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(storeURL))
	if err != nil {
		return ""
	}
	database := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if database == "" {
		return ""
	}
	decoded, err := url.PathUnescape(database)
	if err != nil {
		return database
	}
	return decoded
}
