package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent-testbench/internal/domain/apicasespec"
	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
	"agent-testbench/internal/store/sqlite"
)

const demoDefaultRunPrefix = "demo-create-item"
const demoAPIItemsPath = "/v1/items"

var safeDemoMySQLDatabasePattern = regexp.MustCompile(`(?i)(^|[_-])agent[_-]testbench([_-]|$)|(^|[_-])(smoke|test|ci)([_-]|$)`)

type demoCommandOptions struct {
	outputDir   string
	evidenceDir string
	storeRef    string
	runID       string
	profileID   string
	jsonOutput  bool
	clean       bool
}

type demoOutputRoot struct {
	path    string
	cleanup func() error
	created bool
}

type demoCommandReport struct {
	OK                 bool   `json:"ok"`
	RunID              string `json:"runId"`
	CaseID             string `json:"caseId"`
	Status             string `json:"status"`
	Store              string `json:"store"`
	OutputRoot         string `json:"outputRoot"`
	OutputRetained     bool   `json:"outputRetained"`
	EvidencePath       string `json:"evidencePath"`
	DemoEndpoint       string `json:"demoEndpoint"`
	NextInspectCommand string `json:"nextInspectCommand,omitempty"`
}

func runDemo(ctx context.Context, args []string) error {
	options, err := parseDemoCommandOptions(args)
	if err != nil {
		return err
	}
	report, err := executeDemo(ctx, options)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeIndentedJSON(report)
	}
	printDemoReport(report)
	return nil
}

func parseDemoCommandOptions(args []string) (demoCommandOptions, error) {
	flags := flag.NewFlagSet(cliCommandDemo, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := demoCommandOptions{}
	outputDir := flags.String("output-dir", options.outputDir, "Demo output directory; defaults to a temporary directory")
	evidenceDir := flags.String("evidence-dir", options.evidenceDir, "Evidence output directory; defaults to OUTPUT_DIR/evidence")
	storeRef := flags.String("store", options.storeRef, "Named Store config or Store DSN; defaults to OUTPUT_DIR/store.sqlite")
	runID := flags.String("run-id", options.runID, "Run id; defaults to a demo-create-item timestamp")
	profileID := flags.String("profile", cliCommandDemo, "Profile id for Store records")
	jsonOutput := flags.Bool("json", options.jsonOutput, "Emit a machine-readable JSON report")
	clean := flags.Bool("clean", options.clean, "Remove demo output after a successful run")
	if err := flags.Parse(args); err != nil {
		return demoCommandOptions{}, err
	}
	if flags.NArg() != 0 {
		return demoCommandOptions{}, fmt.Errorf("unexpected demo arguments: %s", strings.Join(flags.Args(), " "))
	}
	options.outputDir = *outputDir
	options.evidenceDir = *evidenceDir
	options.storeRef = *storeRef
	options.runID = *runID
	options.profileID = *profileID
	options.jsonOutput = *jsonOutput
	options.clean = *clean
	return options, nil
}

func executeDemo(ctx context.Context, options demoCommandOptions) (demoCommandReport, error) {
	explicitStoreRef := strings.TrimSpace(options.storeRef) != ""
	explicitEvidenceDir := strings.TrimSpace(options.evidenceDir) != ""
	outputRoot, err := prepareDemoOutputRoot(options.outputDir)
	if err != nil {
		return demoCommandReport{}, err
	}
	if options.clean && !outputRoot.created {
		return demoCommandReport{}, fmt.Errorf("demo refuses --clean with pre-existing --output-dir %s; omit --clean or choose a new demo output directory", outputRoot.path)
	}

	evidenceDir := strings.TrimSpace(options.evidenceDir)
	if evidenceDir == "" {
		evidenceDir = filepath.Join(outputRoot.path, "evidence")
	}
	if err := validateDemoCleanEvidenceRetention(options.clean, explicitStoreRef, explicitEvidenceDir, outputRoot.path, evidenceDir); err != nil {
		return demoCommandReport{}, err
	}
	storeRef := strings.TrimSpace(options.storeRef)
	if storeRef == "" {
		storeRef = "sqlite://" + filepath.Join(outputRoot.path, "store.sqlite")
	}
	storeURL, err := resolveRequiredDailyStoreReference(storeRef, "")
	if err != nil {
		return demoCommandReport{}, err
	}
	if err := requireSafeDemoMySQLStore(storeURL); err != nil {
		return demoCommandReport{}, err
	}
	if err := upgradeDemoStoreSchema(ctx, storeURL); err != nil {
		return demoCommandReport{}, err
	}

	server := httptest.NewServer(http.HandlerFunc(handleDemoAPIRequest))
	defer server.Close()

	casePath, err := writeDemoCase(outputRoot.path)
	if err != nil {
		return demoCommandReport{}, err
	}
	runID := strings.TrimSpace(options.runID)
	if runID == "" {
		runID = defaultDemoRunID(time.Now())
	}
	result, err := apicase.Run(ctx, apicase.RunOptions{
		CasePath:    casePath,
		BaseURL:     server.URL,
		EvidenceDir: evidenceDir,
		RunID:       runID,
	})
	if err != nil {
		return demoCommandReport{}, err
	}
	if err := indexCaseRun(ctx, storeURL, options.profileID, result); err != nil {
		return demoCommandReport{}, err
	}

	maskedStore := maskStoreURL(storeURL)
	nextInspectCommand := ""
	if !options.clean {
		if inspectStoreRef, ok := demoInspectStoreReference(options.storeRef, storeURL); ok {
			nextInspectCommand = fmt.Sprintf(
				"agent-testbench case inspect --view runs --store %s --run %s --json",
				commandline.ShellQuote(inspectStoreRef),
				commandline.ShellQuote(result.RunID),
			)
		}
	}
	report := demoCommandReport{
		OK:                 result.Status == "passed",
		RunID:              result.RunID,
		CaseID:             result.CaseID,
		Status:             result.Status,
		Store:              maskedStore,
		OutputRoot:         outputRoot.path,
		OutputRetained:     !options.clean,
		EvidencePath:       result.EvidencePath,
		DemoEndpoint:       server.URL,
		NextInspectCommand: nextInspectCommand,
	}
	if options.clean {
		if err := outputRoot.cleanup(); err != nil {
			return demoCommandReport{}, err
		}
	}
	return report, nil
}

func defaultDemoRunID(now time.Time) string {
	return demoDefaultRunPrefix + "-" + now.UTC().Format("20060102T150405.000000000")
}

func validateDemoCleanEvidenceRetention(clean bool, explicitStoreRef bool, explicitEvidenceDir bool, outputRoot string, evidenceDir string) error {
	if !clean || !explicitStoreRef {
		return nil
	}
	if !explicitEvidenceDir {
		return fmt.Errorf("demo refuses --clean with an explicit --store unless --evidence-dir points outside the cleaned --output-dir")
	}
	inside, err := demoPathContainsOrEquals(outputRoot, evidenceDir)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("demo refuses --clean because --evidence-dir %s is inside the cleaned --output-dir %s", evidenceDir, outputRoot)
	}
	return nil
}

func demoPathContainsOrEquals(parent string, child string) (bool, error) {
	absoluteParent, err := filepath.Abs(parent)
	if err != nil {
		return false, fmt.Errorf("resolve demo output directory: %w", err)
	}
	absoluteChild, err := filepath.Abs(child)
	if err != nil {
		return false, fmt.Errorf("resolve demo evidence directory: %w", err)
	}
	relative, err := filepath.Rel(absoluteParent, absoluteChild)
	if err != nil {
		return false, fmt.Errorf("compare demo output and evidence directories: %w", err)
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && relative != ".." && !filepath.IsAbs(relative)), nil
}

func prepareDemoOutputRoot(outputDir string) (demoOutputRoot, error) {
	if strings.TrimSpace(outputDir) == "" {
		dir, err := os.MkdirTemp("", "agent-testbench-demo-")
		if err != nil {
			return demoOutputRoot{}, fmt.Errorf("create demo output directory: %w", err)
		}
		return demoOutputRoot{path: dir, cleanup: func() error { return os.RemoveAll(dir) }, created: true}, nil
	}
	absolute, err := filepath.Abs(outputDir)
	if err != nil {
		return demoOutputRoot{}, fmt.Errorf("resolve demo output directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err == nil {
		if !info.IsDir() {
			return demoOutputRoot{}, fmt.Errorf("demo output path exists and is not a directory: %s", absolute)
		}
		return demoOutputRoot{path: absolute, cleanup: func() error { return nil }, created: false}, nil
	}
	if !os.IsNotExist(err) {
		return demoOutputRoot{}, fmt.Errorf("inspect demo output directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return demoOutputRoot{}, fmt.Errorf("create demo output directory: %w", err)
	}
	return demoOutputRoot{path: absolute, cleanup: func() error { return os.RemoveAll(absolute) }, created: true}, nil
}

func demoInspectStoreReference(storeRef string, resolvedStoreURL string) (string, bool) {
	storeRef = strings.TrimSpace(storeRef)
	if storeRef == "" {
		return resolvedStoreURL, true
	}
	if _, err := storeBackendFromURL(storeRef); err != nil {
		return storeRef, true
	}
	if storeURLHasInlinePassword(storeRef) {
		return "", false
	}
	return storeRef, true
}

func storeURLHasInlinePassword(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil {
		return false
	}
	_, ok := parsed.User.Password()
	return ok
}

func requireSafeDemoMySQLStore(storeURL string) error {
	backend, err := storeBackendFromURL(storeURL)
	if err != nil {
		return err
	}
	if backend != "mysql" {
		return nil
	}
	database, err := mysqlStoreDatabaseName(storeURL)
	if err != nil {
		return err
	}
	if !safeDemoMySQLDatabasePattern.MatchString(database) {
		return fmt.Errorf("demo refuses MySQL database %q; use a dedicated sandbox/smoke/test/ci database name", database)
	}
	return nil
}

func mysqlStoreDatabaseName(storeURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(storeURL))
	if err != nil {
		return "", fmt.Errorf("parse MySQL demo Store URL: %w", err)
	}
	escapedPath := strings.TrimLeft(parsed.EscapedPath(), "/")
	database, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", fmt.Errorf("parse MySQL demo Store database path: %w", err)
	}
	database = strings.TrimSpace(database)
	if database == "" {
		return "", errors.New("demo MySQL Store requires a database path")
	}
	return database, nil
}

func upgradeDemoStoreSchema(ctx context.Context, storeURL string) error {
	backend, err := storeBackendFromURL(storeURL)
	if err != nil {
		return err
	}
	switch backend {
	case "sqlite":
		cfg, err := sqlite.ParseConfigFromURL(storeURL)
		if err != nil {
			return err
		}
		_, err = sqlite.UpgradeSchema(ctx, cfg)
		return err
	case "postgres":
		cfg, err := postgres.ParseConfigFromURL(storeURL)
		if err != nil {
			return err
		}
		_, err = postgresUpgradeSchema(ctx, cfg)
		return err
	case "mysql":
		cfg, err := mysql.ParseConfigFromURL(storeURL)
		if err != nil {
			return err
		}
		_, err = mysqlUpgradeSchema(ctx, cfg)
		return err
	default:
		return fmt.Errorf("unsupported demo Store backend: %s", backend)
	}
}

func writeDemoCase(outputRoot string) (string, error) {
	caseDir := filepath.Join(outputRoot, "cases")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return "", fmt.Errorf("create demo case directory: %w", err)
	}
	casePath := filepath.Join(caseDir, "create-item.json")
	item := apicasespec.NewHTTPCase(
		"case.create-item",
		"Create Item",
		http.MethodPost,
		demoAPIItemsPath,
		map[string]string{"Content-Type": "application/json"},
		map[string]any{"id": "item-001", "name": "Example Item"},
		http.StatusCreated,
	)
	item.Assertions.ResponseContains = []string{"created"}
	if err := os.WriteFile(casePath, apicasespec.JSON(item), 0o644); err != nil {
		return "", fmt.Errorf("write demo case: %w", err)
	}
	return casePath, nil
}

func handleDemoAPIRequest(response http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(response, `{"error":"read request body"}`, http.StatusBadRequest)
		return
	}
	if request.Method == http.MethodPost && request.URL.Path == demoAPIItemsPath {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		if _, err := fmt.Fprintf(response, `{"status":"created","received":%s}`, demoJSONBody(body)); err != nil {
			return
		}
		return
	}
	http.NotFound(response, request)
}

func demoJSONBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "null"
	}
	return trimmed
}

func printDemoReport(report demoCommandReport) {
	fmt.Println("AgentTestBench Demo")
	fmt.Printf("Case Run: %s\n", report.RunID)
	fmt.Printf("Case: %s\n", report.CaseID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Evidence bundle: %s\n", report.EvidencePath)
	fmt.Printf("Store: %s\n", report.Store)
	fmt.Printf("Demo endpoint: %s\n", report.DemoEndpoint)
	fmt.Printf("Demo output root: %s\n", report.OutputRoot)
	if report.OutputRetained && strings.TrimSpace(report.NextInspectCommand) != "" {
		fmt.Printf("Next: %s\n", report.NextInspectCommand)
	} else if !report.OutputRetained {
		fmt.Println("Demo output cleanup: enabled")
	}
}
