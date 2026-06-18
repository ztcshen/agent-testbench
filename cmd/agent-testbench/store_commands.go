package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/mysql"
	storeopen "agent-testbench/internal/store/open"
	"agent-testbench/internal/store/postgres"
	"agent-testbench/internal/store/sqlite"
	"agent-testbench/internal/store/sqlstore"
)

var postgresSchemaStatus = postgres.SchemaStatus
var postgresUpgradeSchema = postgres.UpgradeSchema
var mysqlSchemaStatus = mysql.SchemaStatus
var mysqlUpgradeSchema = mysql.UpgradeSchema
var mysqlProvisionDatabase = mysql.ProvisionDatabase

func runStore(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing store command")
	}
	switch args[0] {
	case "config":
		return runStoreConfig(args[1:])
	case "use":
		return runStoreUse(args[1:])
	case "current":
		return runStoreCurrent(args[1:])
	case "ddl":
		return runStoreDDL(args[1:])
	case "copy":
		return runStoreCopy(ctx, args[1:])
	}

	flags := flag.NewFlagSet("store "+args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resolvedStoreURL) == "" {
		return activeStoreRequiredError()
	}
	backend, err := storeBackendFromURL(resolvedStoreURL)
	if err != nil {
		return err
	}
	if backend == "postgres" {
		return runPostgresStoreCommand(ctx, args[0], resolvedStoreURL, *jsonOutput)
	}
	if backend == "mysql" {
		return runMySQLStoreCommand(ctx, args[0], resolvedStoreURL, *jsonOutput)
	}
	return runSQLiteStoreCommand(ctx, args[0], resolvedStoreURL, *jsonOutput)
}

func runPostgresStoreCommand(ctx context.Context, command string, storeURL string, jsonOutput bool) error {
	cfg, err := postgres.ParseConfigFromURL(storeURL)
	if err != nil {
		return err
	}
	switch command {
	case "status":
		status, err := postgresSchemaStatus(ctx, cfg)
		if err != nil {
			if jsonOutput {
				if jsonErr := writeIndentedJSON(postgresStoreStatusErrorReport(cfg.URL, err)); jsonErr != nil {
					return jsonErr
				}
			}
			return err
		}
		if jsonOutput {
			return writeIndentedJSON(postgresStoreStatusReport(status))
		}
		printPostgresStoreStatus(status)
	case "upgrade":
		status, err := postgresUpgradeSchema(ctx, cfg)
		if err != nil {
			return err
		}
		printStoreSchemaUpgrade("URL", maskStoreURL(status.URL), status.CurrentVersion, status.AppliedCount)
	default:
		return fmt.Errorf("unknown store command: %s", command)
	}
	return nil
}

func runMySQLStoreCommand(ctx context.Context, command string, storeURL string, jsonOutput bool) error {
	cfg, err := mysql.ParseConfigFromURL(storeURL)
	if err != nil {
		return err
	}
	switch command {
	case "status":
		return runMySQLStoreStatus(ctx, cfg, jsonOutput)
	case "provision":
		return runMySQLStoreProvision(ctx, cfg, jsonOutput)
	case "upgrade":
		status, err := mysqlUpgradeSchema(ctx, cfg)
		if err != nil {
			return err
		}
		printStoreSchemaUpgrade("URL", maskStoreURL(status.URL), status.CurrentVersion, status.AppliedCount)
	default:
		return fmt.Errorf("unknown store command: %s", command)
	}
	return nil
}

func runMySQLStoreStatus(ctx context.Context, cfg mysql.Config, jsonOutput bool) error {
	status, err := mysqlSchemaStatus(ctx, cfg)
	if err != nil {
		if jsonOutput {
			if jsonErr := writeIndentedJSON(mysqlStoreStatusErrorReport(cfg.URL, err)); jsonErr != nil {
				return jsonErr
			}
		}
		return err
	}
	if jsonOutput {
		return writeIndentedJSON(mysqlStoreStatusReport(status))
	}
	printMySQLStoreStatus(status)
	return nil
}

func runMySQLStoreProvision(ctx context.Context, cfg mysql.Config, jsonOutput bool) error {
	result, err := mysqlProvisionDatabase(ctx, cfg)
	if err != nil {
		if jsonOutput {
			if jsonErr := writeIndentedJSON(map[string]any{
				"ok":      false,
				"backend": "mysql",
				"url":     maskStoreURL(cfg.URL),
				"error":   err.Error(),
			}); jsonErr != nil {
				return jsonErr
			}
		}
		return err
	}
	if jsonOutput {
		return writeIndentedJSON(map[string]any{
			"ok":       true,
			"backend":  "mysql",
			"url":      maskStoreURL(result.URL),
			"database": result.Database,
			"created":  result.Created,
		})
	}
	if result.Created {
		fmt.Printf("Created MySQL store database %s\n", result.Database)
	} else {
		fmt.Printf("MySQL store database already exists: %s\n", result.Database)
	}
	fmt.Printf("URL: %s\n", maskStoreURL(result.URL))
	return nil
}

func runSQLiteStoreCommand(ctx context.Context, command string, storeURL string, jsonOutput bool) error {
	cfg, err := sqlite.ParseConfigFromURL(storeURL)
	if err != nil {
		return err
	}

	switch command {
	case "status":
		status, err := sqlite.SchemaStatus(ctx, cfg)
		if err != nil {
			if jsonOutput {
				if jsonErr := writeIndentedJSON(sqliteStoreStatusErrorReport(cfg, err)); jsonErr != nil {
					return jsonErr
				}
			}
			return err
		}
		if jsonOutput {
			return writeIndentedJSON(sqliteStoreStatusReport(status))
		}
		printStoreStatus(status)
	case "upgrade":
		status, err := sqlite.UpgradeSchema(ctx, cfg)
		if err != nil {
			return err
		}
		printStoreSchemaUpgrade("Path", status.Path, status.CurrentVersion, status.AppliedCount)
	default:
		return fmt.Errorf("unknown store command: %s", command)
	}
	return nil
}

func printStoreSchemaUpgrade(locationLabel string, location string, version int, applied int) {
	fmt.Printf("Upgraded store schema to version %d\n", version)
	fmt.Printf("Applied: %d\n", applied)
	fmt.Printf("%s: %s\n", locationLabel, location)
}

func runStoreDDL(args []string) error {
	flags := flag.NewFlagSet("store ddl", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	backend := flags.String("backend", "", "Store backend")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	selectedBackend := strings.ToLower(strings.TrimSpace(*backend))
	if selectedBackend == "" {
		inferredBackend, err := inferStoreDDLBackend(*storeRef, *storeURL)
		if err != nil {
			return err
		}
		selectedBackend = inferredBackend
	}
	if selectedBackend == "" {
		selectedBackend = "postgres"
	}
	switch selectedBackend {
	case "postgres", "postgresql":
		fmt.Println(strings.Join(sqlstore.SchemaDDL(sqlstore.PostgresDialect{}), "\n\n"))
		return nil
	case "mysql":
		fmt.Println(strings.Join(sqlstore.SchemaDDL(sqlstore.MySQLDialect{}), "\n\n"))
		return nil
	default:
		return fmt.Errorf("unsupported DDL backend %q; supported backends: postgres, mysql", *backend)
	}
}

func inferStoreDDLBackend(storeRef string, legacyStoreURL string) (string, error) {
	storeRef = strings.TrimSpace(storeRef)
	legacyStoreURL = strings.TrimSpace(legacyStoreURL)
	if legacyStoreURL != "" {
		normalized, err := normalizeLegacyStoreURL(legacyStoreURL)
		if err != nil {
			return "", err
		}
		backend, err := storeBackendFromURL(normalized)
		if err != nil {
			return "", err
		}
		return backend, nil
	}
	if storeRef != "" {
		if backend, err := storeBackendFromURL(storeRef); err == nil && backend != "" {
			return backend, nil
		}
		cfg, err := loadStoreConfig()
		if err != nil {
			return "", err
		}
		entry, ok := cfg.Stores[storeRef]
		if !ok {
			return "", fmt.Errorf("store config %q not found", storeRef)
		}
		if strings.TrimSpace(entry.Backend) != "" {
			return strings.ToLower(strings.TrimSpace(entry.Backend)), nil
		}
		return storeBackendFromURL(entry.URL)
	}
	entry, err := activeStoreConfig()
	if err != nil {
		if errors.Is(err, errNoActiveStoreConfigured) {
			return "", nil
		}
		return "", err
	}
	if strings.TrimSpace(entry.Backend) != "" {
		return strings.ToLower(strings.TrimSpace(entry.Backend)), nil
	}
	return storeBackendFromURL(entry.URL)
}

func printStoreStatus(status sqlite.SchemaStatusResult) {
	pending := status.TargetVersion - status.CurrentVersion
	if pending < 0 {
		pending = 0
	}
	fmt.Println("Store: sqlite")
	fmt.Printf("Path: %s\n", status.Path)
	fmt.Printf("Version: %d\n", status.CurrentVersion)
	fmt.Printf("Target: %d\n", status.TargetVersion)
	fmt.Printf("Pending: %d\n", pending)
}

type storeStatusReport struct {
	OK             bool   `json:"ok"`
	Backend        string `json:"backend"`
	URL            string `json:"url,omitempty"`
	Path           string `json:"path,omitempty"`
	CurrentVersion int    `json:"currentVersion"`
	TargetVersion  int    `json:"targetVersion"`
	Pending        int    `json:"pending"`
	Error          string `json:"error,omitempty"`
}

func sqliteStoreStatusReport(status sqlite.SchemaStatusResult) storeStatusReport {
	return storeStatusReport{
		OK:             true,
		Backend:        "sqlite",
		Path:           status.Path,
		CurrentVersion: status.CurrentVersion,
		TargetVersion:  status.TargetVersion,
		Pending:        pendingStoreSchemaVersions(status.CurrentVersion, status.TargetVersion),
	}
}

func sqliteStoreStatusErrorReport(cfg sqlite.Config, statusErr error) storeStatusReport {
	return storeStatusReport{
		OK:      false,
		Backend: "sqlite",
		Path:    cfg.Resolve().Path,
		Error:   statusErr.Error(),
	}
}

func postgresStoreStatusReport(status postgres.SchemaStatusResult) storeStatusReport {
	return storeStatusReport{
		OK:             true,
		Backend:        "postgres",
		URL:            maskStoreURL(status.URL),
		CurrentVersion: status.CurrentVersion,
		TargetVersion:  status.TargetVersion,
		Pending:        pendingStoreSchemaVersions(status.CurrentVersion, status.TargetVersion),
	}
}

func postgresStoreStatusErrorReport(storeURL string, statusErr error) storeStatusReport {
	return storeStatusReport{
		OK:            false,
		Backend:       "postgres",
		URL:           maskStoreURL(storeURL),
		TargetVersion: sqlstore.CurrentSchemaVersion,
		Pending:       sqlstore.CurrentSchemaVersion,
		Error:         statusErr.Error(),
	}
}

func mysqlStoreStatusReport(status mysql.SchemaStatusResult) storeStatusReport {
	return storeStatusReport{
		OK:             true,
		Backend:        "mysql",
		URL:            maskStoreURL(status.URL),
		CurrentVersion: status.CurrentVersion,
		TargetVersion:  status.TargetVersion,
		Pending:        pendingStoreSchemaVersions(status.CurrentVersion, status.TargetVersion),
	}
}

func mysqlStoreStatusErrorReport(storeURL string, statusErr error) storeStatusReport {
	return storeStatusReport{
		OK:            false,
		Backend:       "mysql",
		URL:           maskStoreURL(storeURL),
		TargetVersion: sqlstore.CurrentSchemaVersion,
		Pending:       sqlstore.CurrentSchemaVersion,
		Error:         statusErr.Error(),
	}
}

func pendingStoreSchemaVersions(current int, target int) int {
	pending := target - current
	if pending < 0 {
		return 0
	}
	return pending
}

func openStore(ctx context.Context, storeURL string) (store.Store, error) {
	return storeopen.Open(ctx, storeURL)
}

func runStoreCatalogCase(ctx context.Context, storeURL string, profileID string, caseID string, baseURL string, evidenceDir string, runID string, timeoutSeconds int, overrides map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"caseId":      strings.TrimSpace(caseID),
		"baseUrl":     strings.TrimSpace(baseURL),
		"evidenceDir": strings.TrimSpace(evidenceDir),
		"runId":       strings.TrimSpace(runID),
	}
	if timeoutSeconds > 0 {
		payload["timeoutSeconds"] = timeoutSeconds
	}
	if len(overrides) > 0 {
		payload["overrides"] = overrides
	}
	return runStoreCatalogCaseWithPayload(ctx, storeURL, profileID, payload)
}

func runStoreCatalogCaseWithPayload(ctx context.Context, storeURL string, profileID string, payload map[string]any) (map[string]any, error) {
	if strings.TrimSpace(storeURL) == "" {
		return nil, errNoActiveStoreConfigured
	}
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return nil, err
	}
	defer closeCLIStore(runtime)
	handler := controlplane.NewWithStore(profile.Bundle{ID: strings.TrimSpace(profileID)}, runtime)
	server := httptest.NewServer(handler)
	defer server.Close()
	result, err := postReportMap(server.URL+"/api/test-kit/run", payload)
	if err != nil {
		return nil, err
	}
	status := intFromReportAny(result["httpStatus"])
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("case run failed with http status %d: %s", status, valueString(result["error"]))
	}
	return result, nil
}

func printStoreCatalogCaseRun(result map[string]any) {
	fmt.Printf("Case Run: %s\n", valueString(result["runId"]))
	fmt.Printf("Case: %s\n", valueString(result["caseId"]))
	fmt.Printf("Status: %s\n", valueString(result["status"]))
	if summary := mapFromReportAny(result["summary"]); len(summary) > 0 {
		if target := valueString(summary["targetBaseUrl"]); target != "" {
			fmt.Printf("Target: %s\n", target)
		}
	}
	fmt.Printf("Evidence: %s\n", valueString(result["viewerUrl"]))
}
