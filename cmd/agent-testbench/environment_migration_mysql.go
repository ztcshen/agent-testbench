package main

import (
	"context"
	"strconv"
	"strings"
	"time"
)

func environmentMigrationApplySQL(edge environmentMigrationEdge, item environmentMigrationItem) string {
	var b strings.Builder
	b.WriteString("USE ")
	b.WriteString(mysqlQuoteIdentifier(item.Database))
	b.WriteString(";\n")
	b.WriteString(strings.TrimSpace(item.Content))
	b.WriteString("\n")
	b.WriteString(environmentMigrationInsertHistorySQL(edge, item, "applied"))
	return b.String()
}

func environmentMigrationBaselineSQL(edge environmentMigrationEdge, item environmentMigrationItem) string {
	var b strings.Builder
	b.WriteString("USE ")
	b.WriteString(mysqlQuoteIdentifier(item.Database))
	b.WriteString(";\n")
	b.WriteString(environmentMigrationInsertHistorySQL(edge, item, "baseline"))
	return b.String()
}

func runEnvironmentMigrationWithHistory(ctx context.Context, workspace string, command []string, edge environmentMigrationEdge, item environmentMigrationItem, migrationSQL string, baseline bool) (int, string, string) {
	totalAttempts := 0
	attempts, errText := runRestoreMySQLCommandWithInputRetry(ctx, workspace, command, environmentMigrationEnsureSQL(item))
	totalAttempts += attempts
	if errText != "" {
		return totalAttempts, "", errText
	}
	attempts, output, errText := runRestoreMySQLCommandWithInputOutputRetry(ctx, workspace, command, environmentMigrationHistoryQuerySQL(edge, item))
	totalAttempts += attempts
	if errText != "" {
		return totalAttempts, "", errText
	}
	existing := environmentMigrationChecksumFromOutput(output)
	if existing != "" {
		if existing != item.Checksum {
			return totalAttempts, "", "agent-testbench migration checksum mismatch for " + item.AssetID
		}
		if baseline {
			return totalAttempts, "baseline-already-recorded", ""
		}
		return totalAttempts, "already-applied", ""
	}
	if !baseline {
		for _, precondition := range item.Preconditions {
			query := environmentMigrationPreconditionQuerySQL(item, precondition)
			if query == "" {
				continue
			}
			attempts, output, errText = runRestoreMySQLCommandWithInputOutputRetry(ctx, workspace, command, query)
			totalAttempts += attempts
			if errText != "" {
				return totalAttempts, "", errText
			}
			if environmentMigrationCountFromOutput(output) > 0 {
				return totalAttempts, "", "agent-testbench migration precondition failed for " + item.AssetID + ": " + precondition.Type
			}
		}
	}
	attempts, errText = runRestoreMySQLCommandWithInputRetry(ctx, workspace, command, migrationSQL)
	totalAttempts += attempts
	if errText != "" {
		return totalAttempts, "", errText
	}
	if baseline {
		return totalAttempts, "baseline", ""
	}
	return totalAttempts, "applied", ""
}

func runRestoreMySQLCommandWithInputOutputRetry(ctx context.Context, workdir string, command []string, input string) (int, string, string) {
	const maxAttempts = 60
	const delay = time.Second
	var lastOutput string
	var lastErr string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		output, errText := runRestoreCommandWithInput(ctx, workdir, command, input)
		if errText == "" {
			return attempt, output, ""
		}
		lastOutput = output
		lastErr = errText
		if !environmentRestoreMySQLApplyErrCanRetry(errText) {
			return attempt, output, errText
		}
		if attempt == maxAttempts {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return attempt, lastOutput, ctx.Err().Error()
		case <-timer.C:
		}
	}
	return maxAttempts, lastOutput, lastErr
}

func environmentMigrationChecksumFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		value := strings.TrimSpace(line)
		if len(value) == 64 && looksLikeHex(value) {
			return value
		}
	}
	return ""
}

func environmentMigrationCountFromOutput(output string) int {
	lines := strings.Split(output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		value := strings.TrimSpace(lines[index])
		if value == "" {
			continue
		}
		if count, err := strconv.Atoi(value); err == nil {
			return count
		}
	}
	return 0
}

func looksLikeHex(value string) bool {
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func environmentMigrationEnsureSQL(item environmentMigrationItem) string {
	var b strings.Builder
	b.WriteString("CREATE DATABASE IF NOT EXISTS ")
	b.WriteString(mysqlQuoteIdentifier(item.Database))
	b.WriteString(";\nUSE ")
	b.WriteString(mysqlQuoteIdentifier(item.Database))
	b.WriteString(";\n")
	b.WriteString(environmentMigrationHistoryTableSQL())
	b.WriteString("\n")
	return b.String()
}

func environmentMigrationHistoryQuerySQL(edge environmentMigrationEdge, item environmentMigrationItem) string {
	return "SELECT checksum FROM " + mysqlQualifiedMigrationHistoryTable(item) +
		" WHERE env_id = " + mysqlQuoteLiteral(firstNonEmpty(item.EnvironmentID, "unknown")) +
		" AND owner_component_id = " + mysqlQuoteLiteral(edge.Owner) +
		" AND provider_component_id = " + mysqlQuoteLiteral(edge.Provider) +
		" AND database_name = " + mysqlQuoteLiteral(item.Database) +
		" AND version = " + mysqlQuoteLiteral(item.Version) +
		" LIMIT 1;\n"
}

func environmentMigrationInsertHistorySQL(edge environmentMigrationEdge, item environmentMigrationItem, status string) string {
	var b strings.Builder
	b.WriteString("    INSERT INTO ")
	b.WriteString(mysqlQualifiedMigrationHistoryTable(item))
	b.WriteString(" (env_id, owner_component_id, provider_component_id, database_name, version, asset_id, checksum, status, execution_ms, error) VALUES (")
	b.WriteString(mysqlQuoteLiteral(firstNonEmpty(item.EnvironmentID, "unknown")))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(edge.Owner))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(edge.Provider))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(item.Database))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(item.Version))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(item.AssetID))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(item.Checksum))
	b.WriteString(", ")
	b.WriteString(mysqlQuoteLiteral(status))
	b.WriteString(", 0, '');\n")
	return b.String()
}

func mysqlQualifiedMigrationHistoryTable(item environmentMigrationItem) string {
	return mysqlQuoteIdentifier(item.Database) + "." + mysqlQuoteIdentifier(environmentMigrationHistoryTable)
}

func environmentMigrationHistoryTableSQL() string {
	return `CREATE TABLE IF NOT EXISTS agent_testbench_schema_history (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  env_id VARCHAR(128) NOT NULL,
  owner_component_id VARCHAR(128) NOT NULL,
  provider_component_id VARCHAR(128) NOT NULL,
  database_name VARCHAR(128) NOT NULL,
  version VARCHAR(64) NOT NULL,
  asset_id VARCHAR(256) NOT NULL,
  checksum CHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  installed_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  execution_ms BIGINT NOT NULL DEFAULT 0,
  error TEXT,
  UNIQUE KEY uq_atb_migration (
    env_id, owner_component_id, provider_component_id, database_name, version
  )
);`
}

func environmentMigrationPreconditionQuerySQL(item environmentMigrationItem, precondition environmentMigrationPrecondition) string {
	switch precondition.Type {
	case environmentMigrationPreconditionColumnNotExists:
		return "USE " + mysqlQuoteIdentifier(item.Database) + ";\n" +
			"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = " + mysqlQuoteLiteral(precondition.Table) + " AND COLUMN_NAME = " + mysqlQuoteLiteral(precondition.Column) + ";\n"
	default:
		return ""
	}
}

func mysqlQuoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func mysqlQuoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
