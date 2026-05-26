package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
	"agent-testbench/internal/store/sqlite"
)

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "AGENT_TESTBENCH_TEST_CLI=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent-testbench %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func extractJSONObject(t *testing.T, output string) string {
	t.Helper()
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end < start {
		t.Fatalf("output does not contain a JSON object:\n%s", output)
	}
	return output[start : end+1]
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func configureNamedPostgreSQLActiveStore(t *testing.T, name string) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("AGENT_TESTBENCH_TEST_PG_DSN"))
	if dsn == "" {
		t.Skip("set AGENT_TESTBENCH_TEST_PG_DSN to run named PostgreSQL daily path coverage")
	}
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	runCLI(t, "store", "config", "set", name, "--url", dsn)
	runCLI(t, "store", "use", name)
	runCLI(t, "store", "upgrade")
	return dsn
}

func configureNamedMySQLActiveStore(t *testing.T, name string) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("AGENT_TESTBENCH_MYSQL_TEST_DSN"))
	if dsn == "" {
		t.Skip("set AGENT_TESTBENCH_MYSQL_TEST_DSN to run named MySQL daily path coverage")
	}
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	runCLI(t, "store", "config", "set", name, "--url", dsn)
	runCLI(t, "store", "use", name)
	runCLI(t, "store", "upgrade")
	resetNamedMySQLActiveStore(t, dsn)
	return dsn
}

func configureNamedSQLiteActiveStore(t *testing.T, name string) string {
	t.Helper()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "store.sqlite")
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	runCLI(t, "store", "config", "set", name, "--url", storeRef)
	runCLI(t, "store", "use", name)
	runCLI(t, "store", "upgrade")
	return storeRef
}

func uniqueTestID(t *testing.T, prefix string) string {
	t.Helper()
	slug := strings.ToLower(t.Name())
	slug = strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(slug)
	return fmt.Sprintf("%s.%s.%d", prefix, slug, time.Now().UTC().UnixNano())
}

func resetNamedMySQLActiveStore(t *testing.T, dsn string) {
	t.Helper()
	cfg, err := mysql.ParseConfigFromURL(dsn)
	if err != nil {
		t.Fatalf("parse MySQL test store DSN: %v", err)
	}
	db, err := sql.Open(cfg.DriverName, cfg.DSN)
	if err != nil {
		t.Fatalf("open MySQL test store for reset: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `
select table_name
from information_schema.tables
where table_schema = database()
  and table_type = 'BASE TABLE'
  and table_name <> 'schema_versions'
order by table_name;`)
	if err != nil {
		t.Fatalf("list MySQL test store tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan MySQL test store table: %v", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate MySQL test store tables: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close MySQL test store table cursor: %v", err)
	}
	if _, err := db.ExecContext(ctx, "set foreign_key_checks = 0"); err != nil {
		t.Fatalf("disable MySQL foreign key checks: %v", err)
	}
	defer db.ExecContext(context.Background(), "set foreign_key_checks = 1")
	for _, table := range tables {
		if _, err := db.ExecContext(ctx, "delete from "+quoteMySQLTestIdent(table)); err != nil {
			t.Fatalf("clear MySQL test table %q: %v", table, err)
		}
	}
}

func quoteMySQLTestIdent(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func seedEnvironmentVerificationArtifacts(t *testing.T, storeRef string, runID string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open verification artifact Store: %v", err)
	}
	defer runtime.Close()
	now := time.Now().UTC()
	if _, err := runtime.CreateRun(ctx, store.Run{
		ID:         runID,
		ProfileID:  "sample",
		WorkflowID: "workflow.core-10",
		Status:     store.StatusPassed,
		SummaryJSON: `{"acceptance":{"templateId":"environment.workflow.skywalking.v1","ok":true,"workflowId":"workflow.core-10",
"expectedSteps":1,"completedSteps":1,"passedSteps":1,"failedSteps":0,"topologyProvider":"skywalking",
"steps":[{"stepId":"step.core-10","caseId":"case.core-10","status":"passed","elapsedMs":12,"evidenceComplete":true,"topologyComplete":true}]}}`,
		StartedAt:  now.Add(-time.Second),
		FinishedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("seed verification run: %v", err)
	}
	if _, err := runtime.RecordEvidence(ctx, store.EvidenceRecord{
		ID:         runID + ".summary",
		RunID:      runID,
		Kind:       "summary",
		URI:        "store://verification/" + runID + "/summary.json",
		MediaType:  "application/json",
		SHA256:     "verification-summary-sha256",
		SizeBytes:  2,
		Summary:    `{"status":"passed"}`,
		Category:   "verification",
		Visibility: "internal",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("seed verification Evidence: %v", err)
	}
	if _, err := runtime.SaveTraceTopology(ctx, store.TraceTopology{
		ID:            runID + ".topology.skywalking",
		WorkflowRunID: runID,
		WorkflowID:    "workflow.core-10",
		StepID:        "step.core-10",
		CaseID:        "case.core-10",
		RequestID:     "request.core-10",
		TraceID:       "trace.core-10",
		Status:        "complete",
		TopologyJSON:  `{"provider":"skywalking","status":"complete","traceId":"trace.core-10","spanCount":2,"confirmedEdges":[{"source":"service.entry","target":"service.worker"}],"observedNodes":["service.entry","service.worker"]}`,
		TextTopology:  "service.entry -> service.worker",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("seed verification topology: %v", err)
	}
}

func createBareGitRepo(t *testing.T, branch string) string {
	return createBareGitRepoWithFiles(t, branch, map[string]string{
		"README.md": "# restore fixture\n",
	})
}

func createBareGitRepoWithFiles(t *testing.T, branch string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	work := filepath.Join(dir, "work")
	runGit(t, "", "init", "--bare", remote)
	runGit(t, "", "init", "-b", branch, work)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeFile(t, filepath.Join(work, name), files[name])
	}
	runGit(t, work, "add", ".")
	runGit(t, work, "-c", "user.name=Open Test", "-c", "user.email=open-test@example.com", "commit", "-m", "initial")
	runGit(t, work, "remote", "add", "origin", remote)
	runGit(t, work, "push", "origin", branch)
	return remote
}

func runGit(t *testing.T, workdir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func fakeDockerCommand(t *testing.T) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	callsPath := filepath.Join(dir, "docker-calls.txt")
	dockerPath := filepath.Join(dir, "docker")
	writeFile(t, dockerPath, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DOCKER_CALLS_FILE\"\nif [ \"$1\" = \"compose\" ] && [ \"$2\" = \"version\" ]; then\n  printf 'Docker Compose version v2.0.0\\n'\n  exit 0\nfi\nif [ \"$1\" = \"compose\" ]; then\n  prev=\"\"\n  service=\"\"\n  for arg in \"$@\"; do\n    if [ \"$prev\" = \"--format\" ] && [ \"$arg\" = \"json\" ]; then\n      service=\"__next__\"\n    elif [ \"$service\" = \"__next__\" ]; then\n      service=\"$arg\"\n    fi\n    prev=\"$arg\"\n  done\n  if [ -n \"$service\" ] && [ \"$service\" != \"__next__\" ]; then\n    printf '{\"Name\":\"%s\",\"Service\":\"%s\",\"State\":\"running\",\"Health\":\"healthy\"}\\n' \"$service\" \"$service\"\n  fi\nfi\n")
	if err := os.Chmod(dockerPath, 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	return []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"DOCKER_CALLS_FILE=" + callsPath,
	}, callsPath
}

func newHealthyTestURL(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/health"
}

func fakeMySQLApplyCommandWithFirstFailure(t *testing.T) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	callsPath := filepath.Join(dir, "mysql-apply-calls.txt")
	statePath := filepath.Join(dir, "mysql-exec-attempts.txt")
	commandPath := filepath.Join(dir, "mysql-apply")
	writeFile(t, commandPath, `#!/usr/bin/env bash
set -euo pipefail
printf 'apply\n' >> "$MYSQL_APPLY_CALLS_FILE"
attempts=0
if [[ -f "$MYSQL_EXEC_ATTEMPTS_FILE" ]]; then
  attempts=$(cat "$MYSQL_EXEC_ATTEMPTS_FILE")
fi
attempts=$((attempts + 1))
printf '%s\n' "$attempts" > "$MYSQL_EXEC_ATTEMPTS_FILE"
if [[ "$attempts" -eq 1 ]]; then
  printf "mysql: [Warning] Using a password on the command line interface can be insecure.\nERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)\n" >&2
  exit 1
fi
cat >/dev/null
`)
	if err := os.Chmod(commandPath, 0o755); err != nil {
		t.Fatalf("chmod fake mysql apply command: %v", err)
	}
	t.Setenv("MYSQL_APPLY_CALLS_FILE", callsPath)
	t.Setenv("MYSQL_EXEC_ATTEMPTS_FILE", statePath)
	return []string{commandPath}, callsPath
}

func installGitRemoteFixture(t *testing.T, binDir string, remoteURL string, fixtureRepo string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
remote_url=%q
fixture_repo=%q
args=()
for arg in "$@"; do
  if [[ "$arg" == "$remote_url" ]]; then
    args+=("$fixture_repo")
  else
    args+=("$arg")
  fi
done
exec %q "${args[@]}"
`, remoteURL, fixtureRepo, realGit)
	gitPath := filepath.Join(binDir, "git")
	writeFile(t, gitPath, script)
	if err := os.Chmod(gitPath, 0o755); err != nil {
		t.Fatalf("chmod fake git: %v", err)
	}
}

func runCLIWithEnv(t *testing.T, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), env...), "AGENT_TESTBENCH_TEST_CLI=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent-testbench %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runCLIFailsWithEnv(t *testing.T, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), env...), "AGENT_TESTBENCH_TEST_CLI=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("agent-testbench %s unexpectedly succeeded:\n%s", strings.Join(args, " "), out)
	}
	return string(out)
}

func runStoreCommand(t *testing.T, args ...string) string {
	t.Helper()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	originalStdout := os.Stdout
	os.Stdout = writePipe
	runErr := runStore(context.Background(), args)
	if closeErr := writePipe.Close(); closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}
	os.Stdout = originalStdout
	out, readErr := io.ReadAll(readPipe)
	if readErr != nil {
		t.Fatalf("read stdout pipe: %v", readErr)
	}
	if runErr != nil {
		t.Fatalf("store %s failed: %v\n%s", strings.Join(args, " "), runErr, out)
	}
	return string(out)
}

func runStoreCommandFails(t *testing.T, args ...string) string {
	t.Helper()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	originalStdout := os.Stdout
	os.Stdout = writePipe
	runErr := runStore(context.Background(), args)
	if closeErr := writePipe.Close(); closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}
	os.Stdout = originalStdout
	out, readErr := io.ReadAll(readPipe)
	if readErr != nil {
		t.Fatalf("read stdout pipe: %v", readErr)
	}
	if runErr == nil {
		t.Fatalf("store %s unexpectedly succeeded:\n%s", strings.Join(args, " "), out)
	}
	return string(out)
}

func withPostgresSchemaStatus(t *testing.T, fn func(context.Context, postgres.Config) (postgres.SchemaStatusResult, error)) {
	t.Helper()
	original := postgresSchemaStatus
	postgresSchemaStatus = fn
	t.Cleanup(func() {
		postgresSchemaStatus = original
	})
}

func withMySQLSchemaStatus(t *testing.T, fn func(context.Context, mysql.Config) (mysql.SchemaStatusResult, error)) {
	t.Helper()
	original := mysqlSchemaStatus
	mysqlSchemaStatus = fn
	t.Cleanup(func() {
		mysqlSchemaStatus = original
	})
}

func withMySQLProvisionDatabase(t *testing.T, fn func(context.Context, mysql.Config) (mysql.ProvisionDatabaseResult, error)) {
	t.Helper()
	original := mysqlProvisionDatabase
	mysqlProvisionDatabase = fn
	t.Cleanup(func() {
		mysqlProvisionDatabase = original
	})
}

func sqliteScalar(t *testing.T, dbPath string, statement string) string {
	t.Helper()
	out, err := exec.Command("sqlite3", dbPath, statement).CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite scalar failed: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func hasProfileVerifyCheck(checks []struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}, name string) bool {
	for _, check := range checks {
		if check.Name == name && check.OK {
			return true
		}
	}
	return false
}

func hasReadModels(readModels []string, required ...string) bool {
	seen := map[string]bool{}
	for _, key := range readModels {
		seen[key] = true
	}
	for _, key := range required {
		if !seen[key] {
			return false
		}
	}
	return true
}

func firstJSONObject(t *testing.T, out string) string {
	t.Helper()
	start := strings.Index(out, "{")
	end := strings.LastIndex(out, "\n}")
	if end < 0 {
		end = strings.LastIndex(out, "}")
	} else {
		end++
	}
	if start < 0 || end < start {
		t.Fatalf("output does not contain a JSON object:\n%s", out)
	}
	return out[start : end+1]
}

func runCLIFails(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "AGENT_TESTBENCH_TEST_CLI=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("agent-testbench %s unexpectedly succeeded:\n%s", strings.Join(args, " "), out)
	}
	return string(out)
}

func readTarGZEntries(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open archive %s: %v", path, err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip %s: %v", path, err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var entries []string
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read archive %s: %v", path, err)
		}
		entries = append(entries, header.Name)
	}
	return entries
}

func writeTarGZEntries(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive %s: %v", path, err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	writer := tar.NewWriter(gz)
	defer writer.Close()
	for name, body := range entries {
		header := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write archive header %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatalf("write archive entry %s: %v", name, err)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func createStoredCaseRun(t *testing.T, runID string, label string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	evidenceDir := filepath.Join(dir, "evidence")

	runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", runID, "--evidence-dir", evidenceDir, "--profile", "sample")
	t.Logf("created %s stored case run %s", label, runID)
}

func createPostProcessTaskStore(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open post process task store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("close post process task store: %v", err)
		}
	})
	seedPostProcessTaskFixture(t, ctx, s, "run.tasks", "")
	return storePath
}

func seedPostProcessTaskFixture(t *testing.T, ctx context.Context, s store.Store, runID string, idPrefix string) {
	t.Helper()
	base := time.Date(2026, 5, 17, 1, 2, 3, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         runID,
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusPassed,
		StartedAt:  base,
		FinishedAt: base.Add(time.Second),
		CreatedAt:  base,
		UpdatedAt:  base.Add(time.Second),
	}); err != nil {
		t.Fatalf("create task run: %v", err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:         idPrefix + "evidence.response",
		RunID:      runID,
		CaseRunID:  runID + ".case",
		StepID:     "step-a",
		Kind:       "response",
		URI:        "store://evidence/" + runID + "/response.json",
		MediaType:  "application/json",
		SHA256:     "response-sha256",
		SizeBytes:  2,
		Summary:    `{"statusCode":200}`,
		Category:   "http",
		Visibility: "internal",
		CreatedAt:  base.Add(5 * time.Millisecond),
	}); err != nil {
		t.Fatalf("record task Evidence: %v", err)
	}
	records := []store.PostProcessTask{
		{
			ID:         idPrefix + "task.trace",
			RunID:      runID,
			WorkflowID: "workflow.alpha",
			StepID:     "step-a",
			CaseID:     "case.alpha",
			Kind:       "trace_topology_collect",
			Status:     store.StatusPassed,
			StartedAt:  base.Add(10 * time.Millisecond),
			FinishedAt: base.Add(135 * time.Millisecond),
			CreatedAt:  base.Add(10 * time.Millisecond),
		},
		{
			ID:          idPrefix + "task.logs",
			RunID:       runID,
			WorkflowID:  "workflow.alpha",
			StepID:      "step-b",
			CaseID:      "case.beta",
			Kind:        "runtime_log_collect",
			Status:      store.StatusFailed,
			StartedAt:   base.Add(200 * time.Millisecond),
			FinishedAt:  base.Add(500 * time.Millisecond),
			Error:       "log source missing",
			SummaryJSON: `{"source":"runtime-log"}`,
			CreatedAt:   base.Add(200 * time.Millisecond),
		},
		{
			ID:          idPrefix + "task.trace.skip",
			RunID:       runID,
			WorkflowID:  "workflow.alpha",
			StepID:      "step-c",
			CaseID:      "case.gamma",
			Kind:        "trace_topology_collect",
			Status:      store.StatusSkipped,
			StartedAt:   base.Add(600 * time.Millisecond),
			FinishedAt:  base.Add(600 * time.Millisecond),
			SummaryJSON: `{"reason":"SkyWalking provider unavailable"}`,
			CreatedAt:   base.Add(600 * time.Millisecond),
		},
	}
	for _, record := range records {
		if _, err := s.RecordPostProcessTask(ctx, record); err != nil {
			t.Fatalf("record post process task %s: %v", record.ID, err)
		}
	}
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

func jsonPrefix(output string) string {
	if index := strings.LastIndex(output, "\n}"); index >= 0 {
		return output[:index+2]
	}
	return output
}

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

func writeCaseSuiteCoverageProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile.json"), `{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [],
  "interfaceNodes": [{"id":"node.alpha","displayName":"Node Alpha","serviceId":"service.alpha","operation":"Alpha","method":"GET","path":"/alpha"}],
  "apiCases": [
    {"id":"case.default","displayName":"Default Case","nodeId":"node.alpha","sortOrder":1,"tags":["regression","smoke"],"priority":"p0","owner":"team-a","description":"Default maintained case.","casePath":"cases/default.json"},
    {"id":"case.variant","displayName":"Variant Case","nodeId":"node.alpha","sortOrder":2,"tags":["regression"],"priority":"p1","owner":"team-a","description":"Variant maintained case."},
    {"id":"case.unrun","displayName":"Unrun Case","nodeId":"node.alpha","sortOrder":3,"tags":["regression"],"priority":"p2","owner":"team-b","description":"Unrun maintained case."}
  ],
  "requestTemplates": [],
  "templateConfigs": [
    {
      "id": "config.case.variant",
      "scopeType": "case",
      "scopeId": "case.variant",
      "status": "active",
      "configJson": "{\"caseId\":\"case.variant\",\"caseExecution\":{\"method\":\"GET\",\"nodeId\":\"node.alpha\",\"path\":\"/alpha\",\"expectedHttpCodes\":[200]}}"
    }
  ],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`)
	return dir
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

func recordCaseRunForCoverage(t *testing.T, ctx context.Context, s store.Store, runID string, caseID string, status string, at time.Time) {
	t.Helper()
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         runID,
		ProfileID:  "sample",
		WorkflowID: caseID,
		Status:     status,
		StartedAt:  at,
		FinishedAt: at.Add(time.Second),
		CreatedAt:  at,
		UpdatedAt:  at.Add(time.Second),
	}); err != nil {
		t.Fatalf("create coverage run %s: %v", runID, err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:         runID + ".case",
		RunID:      runID,
		CaseID:     caseID,
		Status:     status,
		StartedAt:  at,
		FinishedAt: at.Add(time.Second),
		CreatedAt:  at,
	}); err != nil {
		t.Fatalf("record coverage case run %s: %v", runID, err)
	}
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

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json file %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode json file %s: %v\n%s", path, err, raw)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func createLegacyRuntimeDB(t *testing.T, path string) {
	t.Helper()
	createLegacyRuntimeDBWithIDs(t, path, 7, 11, "case-run-parent")
}

func createLegacyRuntimeDBWithIDs(t *testing.T, path string, workflowLegacyID int64, caseLegacyID int64, parentRunID string) {
	t.Helper()
	statement := fmt.Sprintf(`
create table workflow_runs (
  id integer primary key,
  workflow_id text not null,
  status text not null,
  summary_json text not null default '',
  created_at text not null
);
create table interface_node_case_run (
  id integer primary key,
  node_id text not null,
  case_id text not null,
  run_id text not null,
  status text not null,
  failure_kind text not null default '',
  failure_reason text not null default '',
  evidence_path text not null default '',
  elapsed_ms integer not null default 0,
  summary_json text not null default '',
  created_at text not null
);
insert into workflow_runs(id, workflow_id, status, summary_json, created_at)
values (%d, 'workflow.alpha', 'passed', '{"steps":1}', '2026-05-14T01:02:03Z');
insert into interface_node_case_run(id, node_id, case_id, run_id, status, evidence_path, summary_json, created_at)
values (%d, 'node.alpha', 'case.alpha', '%s', 'failed', '.runtime/cases/%s', '{"failure":"expected"}', '2026-05-14T01:03:03Z');
`, workflowLegacyID, caseLegacyID, parentRunID, parentRunID)
	cmd := exec.Command("sqlite3", path, statement)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create legacy db: %v\n%s", err, out)
	}
}
