package main

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func environmentRestoreApplyMySQLSQLEdgeAsset(ctx context.Context, content string, contentErr error, workspace string, execute bool, composeBaseArgs []string, options environmentRestoreApplyAssetOptions, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
	item.Action = "plan-apply-mysql-sql"
	item.Command = environmentRestoreMySQLApplyCommand(composeBaseArgs, item.TargetComposeService)
	if len(composeBaseArgs) == 0 || item.TargetComposeService == "" {
		item.OK = false
		item.Error = "mysql edge asset requires a Docker Compose target service"
		return item
	}
	if execute && options.UseExistingContainers {
		item.Action = "skip-mysql-sql-use-existing-containers"
		item.Command = nil
		item.Status = "skipped"
		item.Error = "plain MySQL SQL bootstrap asset was not re-applied to existing containers; convert it to an environment migration asset or rerun restore with a clean Docker state when it must be applied"
		return item
	}
	if contentErr != nil {
		item.OK = false
		item.Error = contentErr.Error()
		return item
	}
	if strings.TrimSpace(content) == "" {
		item.OK = false
		item.Error = "mysql edge asset requires SQL content"
		return item
	}
	if execute {
		item.Action = "apply-mysql-sql"
		attempts, errText := runRestoreMySQLCommandWithInputRetry(ctx, workspace, item.Command, content)
		item.Attempts = attempts
		if errText != "" {
			item.OK = false
			item.Error = errText
		}
	}
	return item
}

func environmentRestoreIsMySQLSQLAsset(asset store.ComponentConfigAsset, dep store.ComponentDependency) bool {
	kind := strings.ToLower(strings.TrimSpace(asset.AssetKind))
	capability := strings.ToLower(strings.TrimSpace(dep.Capability))
	if kind == "" {
		return false
	}
	tokens := strings.FieldsFunc(kind, func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	hasSQLToken := false
	hasMySQLToken := false
	for _, token := range tokens {
		switch token {
		case "ddl", "schema", "seed", "sql":
			hasSQLToken = true
		case "mysql":
			hasMySQLToken = true
		}
	}
	if !hasSQLToken {
		return false
	}
	if hasMySQLToken {
		return true
	}
	return capability == "sql" && (environmentRestoreHasMySQLComponentSignal(asset.TargetComponentID) || environmentRestoreHasMySQLComponentSignal(dep.ProviderComponentID))
}

func environmentRestoreHasMySQLComponentSignal(componentID string) bool {
	tokens := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(componentID)), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	for _, token := range tokens {
		if token == "mysql" {
			return true
		}
	}
	return false
}

func environmentRestoreMySQLApplyCommand(composeBaseArgs []string, service string) []string {
	command := append([]string{"docker", "compose"}, composeBaseArgs...)
	command = append(command, "exec", "-T", service, "sh", "-lc", environmentRestoreMySQLClientScript())
	return command
}

func environmentRestoreMySQLClientScript() string {
	return `user="${MYSQL_USER:-root}"
password="${MYSQL_PASSWORD:-${MYSQL_ROOT_PASSWORD:-}}"
database="${AGENT_TESTBENCH_MYSQL_APPLY_DATABASE:-}"
set --
if [ -n "$user" ]; then
  set -- "$@" "-u${user}"
fi
if [ -n "$password" ]; then
  set -- "$@" "-p${password}"
fi
if [ -n "$database" ]; then
  set -- "$@" "${database}"
fi
exec mysql "$@"`
}

func runRestoreMySQLCommandWithInputRetry(ctx context.Context, workdir string, command []string, input string) (int, string) {
	const maxAttempts = 60
	const delay = time.Second
	var lastErr string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, errText := runRestoreCommandWithInput(ctx, workdir, command, input)
		if errText == "" {
			return attempt, ""
		}
		lastErr = errText
		if !environmentRestoreMySQLApplyErrCanRetry(errText) {
			return attempt, errText
		}
		if attempt == maxAttempts {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return attempt, ctx.Err().Error()
		case <-timer.C:
		}
	}
	return maxAttempts, lastErr
}

func environmentRestoreMySQLApplyErrCanRetry(errText string) bool {
	lower := strings.ToLower(errText)
	retryable := []string{
		"access denied for user 'root'@'localhost'",
		"can't connect to local mysql server",
		"can't connect to mysql server",
		"lost connection to mysql server",
		"server has gone away",
		"error 1045",
		"error 2002",
		"error 2003",
		"error 2013",
	}
	for _, item := range retryable {
		if strings.Contains(lower, item) {
			return true
		}
	}
	return false
}
