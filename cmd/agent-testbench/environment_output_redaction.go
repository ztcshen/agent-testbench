package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"agent-testbench/internal/domain/redaction"
)

type environmentOutputRedactorContextKey struct{}

const environmentSensitiveTokenSecret = "secret"

type environmentOutputRedactor struct {
	secrets []string
}

func contextWithEnvironmentOutputRedactor(ctx context.Context, sources ...any) context.Context {
	return context.WithValue(ctx, environmentOutputRedactorContextKey{}, newEnvironmentOutputRedactor(sources...))
}

func environmentOutputRedactorFromContext(ctx context.Context) (environmentOutputRedactor, bool) {
	value, ok := ctx.Value(environmentOutputRedactorContextKey{}).(environmentOutputRedactor)
	return value, ok
}

func newEnvironmentOutputRedactor(sources ...any) environmentOutputRedactor {
	seen := map[string]bool{}
	for _, source := range sources {
		collectEnvironmentOutputSecrets(environmentOutputAny(source), "", seen)
	}
	secrets := make([]string, 0, len(seen))
	for secret := range seen {
		secrets = append(secrets, secret)
	}
	sort.Slice(secrets, func(i int, j int) bool {
		return len(secrets[i]) > len(secrets[j])
	})
	return environmentOutputRedactor{secrets: secrets}
}

func environmentRestoreReportForOutput(report environmentRestoreReport) environmentRestoreReport {
	phase := environmentRestorePhase(report)
	report.Docker.Output = environmentLifecycleSafeOperationalOutput(report.Docker.Output)
	report.Docker.Cleanup.Output = nil
	report.Docker.HealthChecks = environmentLifecycleHealthChecksForOutput(report.Docker.HealthChecks)
	if report.Docker.Error != "" && phase == "health-check" {
		report.Docker.Error = "environment health check did not pass"
		report.Error = report.Docker.Error
	} else if report.Docker.Error != "" && phase == "docker" && environmentLifecycleActionRanCommand(report.Docker.Action) {
		report.Docker.Error = environmentLifecycleCommandFailureMessage(report.Docker.Action)
		if report.Error != "" {
			report.Error = report.Docker.Error
		}
	}
	if report.Docker.Cleanup.Error != "" && report.Docker.Cleanup.Action == "run-cleanup" {
		report.Docker.Cleanup.Error = environmentLifecycleCommandFailureMessage("docker cleanup")
		report.Docker.Error = report.Docker.Cleanup.Error
		report.Error = report.Docker.Error
	}
	return environmentOutputRedacted(report, report)
}

func environmentStatusReportForOutput(report environmentStatusReport, compose map[string]any) environmentStatusReport {
	report.Docker.HealthChecks = environmentLifecycleHealthChecksForOutput(report.Docker.HealthChecks)
	commandFailure := false
	for _, check := range report.Docker.HealthChecks {
		if check.Kind == "command" && !check.OK {
			commandFailure = true
		}
	}
	for index := range report.Docker.Services {
		if report.Docker.Services[index].Service == "docker compose ps" && report.Docker.Services[index].Error != "" {
			report.Docker.Services[index].Error = "environment service is not ready"
			commandFailure = true
		}
	}
	if report.Docker.Error != "" && report.Docker.Action == environmentLifecycleActionInspectStatusCommand {
		if len(report.Docker.HealthChecks) > 0 {
			report.Docker.Error = environmentLifecycleCommandExitMessage("statusCommand", report.Docker.HealthChecks[0].ExitCode)
		} else {
			report.Docker.Error = environmentLifecycleCommandFailureMessage(report.Docker.Action)
		}
		report.Error = report.Docker.Error
	} else if report.Docker.Error != "" && commandFailure {
		report.Docker.Error = environmentLifecycleCommandFailureMessage(report.Docker.Action)
		report.Error = report.Docker.Error
	}
	return environmentOutputRedacted(report, report, compose)
}

func environmentStopReportForOutput(report environmentStopReport, compose map[string]any) environmentStopReport {
	if report.Docker.Action == environmentStopActionCommand {
		report.Docker.Command = nil
	}
	report.Docker.Output = ""
	commandFailed := report.Docker.Action == environmentStopActionCommand || len(report.Docker.Command) > 0
	if report.Docker.Error != "" && commandFailed {
		report.Docker.Error = environmentLifecycleCommandFailureMessage(report.Docker.Action)
		report.Error = report.Docker.Error
	}
	return environmentOutputRedacted(report, report, compose)
}

func environmentPayloadForOutput(payload map[string]any) map[string]any {
	return environmentOutputRedacted(payload, payload)
}

func environmentLifecycleHealthChecksForOutput(checks []environmentRestoreHealthCheckReport) []environmentRestoreHealthCheckReport {
	out := make([]environmentRestoreHealthCheckReport, len(checks))
	copy(out, checks)
	for index := range out {
		out[index].Output = ""
		if out[index].Kind == "command" {
			out[index].Command = ""
		}
		if out[index].Error != "" {
			out[index].Error = "environment health check did not pass"
		}
	}
	return out
}

func environmentLifecycleSafeOperationalOutput(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(strings.TrimSpace(value), "generated compose env file: ") {
			out = append(out, value)
		}
	}
	return out
}

func environmentLifecycleActionRanCommand(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	return strings.HasPrefix(action, "run-") || strings.HasPrefix(action, "inspect-") || strings.HasPrefix(action, "compose-")
}

func environmentLifecycleCommandFailureMessage(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "environment lifecycle"
	}
	return action + " command did not complete"
}

func environmentLifecycleCommandExitMessage(action string, exitCode int) string {
	return strings.TrimSpace(action) + " exited with code " + fmt.Sprint(exitCode)
}

func environmentStreamEventForOutput(ctx context.Context, event agentStreamEvent) agentStreamEvent {
	redactor, ok := environmentOutputRedactorFromContext(ctx)
	if !ok {
		return event
	}
	return environmentOutputRedactedWith(redactor, event)
}

func environmentOutputRedacted[T any](value T, sources ...any) T {
	return environmentOutputRedactedWith(newEnvironmentOutputRedactor(sources...), value)
}

func environmentOutputRedactedWith[T any](redactor environmentOutputRedactor, value T) T {
	var zero T
	raw, err := json.Marshal(value)
	if err != nil {
		return zero
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return zero
	}
	redacted := redactor.redactValue(decoded, "")
	raw, err = json.Marshal(redacted)
	if err != nil {
		return zero
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero
	}
	return out
}

func environmentOutputAny(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func collectEnvironmentOutputSecrets(value any, key string, seen map[string]bool) {
	normalized := normalizeEnvironmentOutputKey(key)
	if normalized == "generatedfiles" {
		if files, ok := value.(map[string]any); ok {
			collectEnvironmentGeneratedFileSecrets(files, seen)
		}
	}
	if isEnvironmentOutputSensitiveKey(normalized) {
		collectEnvironmentOutputStringValues(value, seen)
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			collectEnvironmentOutputSecrets(child, childKey, seen)
		}
	case []any:
		for _, child := range typed {
			collectEnvironmentOutputSecrets(child, "", seen)
		}
	case string:
		if isEnvironmentOutputURLKey(normalized) {
			collectEnvironmentOutputURLSecrets(typed, seen)
		}
	}
}

func collectEnvironmentGeneratedFileSecrets(files map[string]any, seen map[string]bool) {
	for _, raw := range files {
		content, ok := raw.(string)
		if !ok {
			continue
		}
		var decoded any
		if json.Unmarshal([]byte(content), &decoded) == nil {
			collectEnvironmentOutputSecrets(decoded, "", seen)
		}
		for _, line := range strings.Split(content, "\n") {
			key, value, ok := environmentGeneratedFileAssignment(line)
			if !ok || !isEnvironmentOutputSensitiveKey(normalizeEnvironmentOutputKey(key)) {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if value != "" && value != redaction.Mask {
				seen[value] = true
			}
		}
	}
}

func collectEnvironmentOutputStringValues(value any, seen map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			collectEnvironmentOutputStringValues(child, seen)
		}
	case []any:
		for _, child := range typed {
			collectEnvironmentOutputStringValues(child, seen)
		}
	case string:
		if typed != "" && typed != redaction.Mask {
			seen[typed] = true
		}
	}
}

func collectEnvironmentOutputURLSecrets(value string, seen map[string]bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return
	}
	if parsed.User != nil {
		if password, ok := parsed.User.Password(); ok && password != "" {
			seen[password] = true
		}
	}
	for key, values := range parsed.Query() {
		if !isEnvironmentOutputSensitiveKey(normalizeEnvironmentOutputKey(key)) {
			continue
		}
		for _, item := range values {
			if item != "" {
				seen[item] = true
			}
		}
	}
}

func (r environmentOutputRedactor) redactValue(value any, key string) any {
	normalized := normalizeEnvironmentOutputKey(key)
	if normalized == "env" {
		if _, ok := value.(map[string]any); ok {
			return redactEnvironmentOutputContainer(value)
		}
	}
	if normalized == "generatedfiles" {
		if files, ok := value.(map[string]any); ok {
			return r.redactGeneratedFiles(files)
		}
	}
	if isEnvironmentOutputSensitiveKey(normalized) || isEnvironmentOutputCommandKey(normalized) {
		switch value.(type) {
		case string, map[string]any, []any:
		default:
			return value
		}
		return redactEnvironmentOutputContainer(value)
	}
	if normalized == "command" {
		return r.redactCommand(value)
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, child := range typed {
			out[childKey] = r.redactValue(child, childKey)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for index, child := range typed {
			if index >= 2 && len(typed) >= 2 && valueString(typed[1]) == "-c" {
				out = append(out, redaction.Mask)
				continue
			}
			out = append(out, r.redactValue(child, ""))
		}
		return out
	case string:
		redacted := typed
		if isEnvironmentOutputURLKey(normalized) {
			redacted = redactEnvironmentOutputURL(redacted)
		}
		redacted = r.replaceSecrets(redacted)
		return redaction.Text(redacted)
	default:
		return value
	}
}

func (r environmentOutputRedactor) redactGeneratedFiles(files map[string]any) map[string]any {
	out := make(map[string]any, len(files))
	for path, raw := range files {
		content, ok := raw.(string)
		if !ok {
			out[path] = r.redactValue(raw, "")
			continue
		}
		out[path] = r.redactGeneratedFileContent(content)
	}
	return out
}

func (r environmentOutputRedactor) redactGeneratedFileContent(content string) string {
	content = r.replaceSecrets(content)
	var decoded any
	if json.Unmarshal([]byte(content), &decoded) == nil {
		return redaction.Text(content)
	}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		key, _, ok := environmentGeneratedFileAssignment(line)
		if !ok || !isEnvironmentOutputSensitiveKey(normalizeEnvironmentOutputKey(key)) {
			continue
		}
		separator := strings.IndexAny(line, "=:")
		lines[index] = line[:separator+1] + redaction.Mask
	}
	return strings.Join(lines, "\n")
}

func (r environmentOutputRedactor) replaceSecrets(value string) string {
	for _, secret := range r.secrets {
		value = strings.ReplaceAll(value, secret, redaction.Mask)
	}
	return value
}

func environmentGeneratedFileAssignment(line string) (string, string, bool) {
	separator := strings.IndexAny(line, "=:")
	if separator <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:separator])
	if key == "" || strings.ContainsAny(key, " /\\") {
		return "", "", false
	}
	return key, line[separator+1:], true
}

func (r environmentOutputRedactor) redactCommand(value any) any {
	switch typed := value.(type) {
	case []any:
		out := make([]any, 0, len(typed))
		for index, item := range typed {
			if index >= 2 && len(typed) >= 2 && valueString(typed[1]) == "-c" {
				out = append(out, redaction.Mask)
				continue
			}
			out = append(out, r.redactValue(item, ""))
		}
		return out
	case string:
		return redaction.Mask
	default:
		return r.redactValue(value, "")
	}
}

func redactEnvironmentOutputContainer(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key := range typed {
			out[key] = redaction.Mask
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = redaction.Mask
		}
	case nil:
		return nil
	}
	return redaction.Mask
}

func redactEnvironmentOutputURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return redaction.URL(value)
	}
	if parsed.User != nil {
		if _, ok := parsed.User.Password(); ok {
			parsed.User = url.UserPassword(parsed.User.Username(), redaction.Mask)
		}
	}
	return redaction.URL(parsed.String())
}

func normalizeEnvironmentOutputKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func isEnvironmentOutputSensitiveKey(normalized string) bool {
	for _, candidate := range []string{
		"accesstoken",
		"apikey",
		"authorization",
		"clientsecret",
		"cookie",
		"credential",
		"password",
		"privatekey",
		environmentSensitiveTokenSecret,
		"setcookie",
		"token",
	} {
		if normalized == candidate || strings.Contains(normalized, candidate) {
			return true
		}
	}
	return false
}

func isEnvironmentOutputCommandKey(normalized string) bool {
	switch normalized {
	case "startcommand", "statuscommand", "stopcommand":
		return true
	default:
		return false
	}
}

func isEnvironmentOutputURLKey(normalized string) bool {
	switch normalized {
	case "endpoint", "fullurl", "operation", "reporturl", "uri", "url":
		return true
	default:
		return false
	}
}
