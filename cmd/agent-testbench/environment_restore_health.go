package main

import (
	"encoding/json"
	"strings"

	"agent-testbench/internal/store"
)

const composeServicesHeader = "services:"

func environmentRestoreEffectiveHealthChecks(checks []any, compose map[string]any, graph store.EnvironmentComponentGraph, workspace string) []any {
	set := environmentRestoreHealthCheckSet{
		covered: map[string]bool{},
		seen:    map[string]bool{},
	}
	startedServices := environmentRestoreStartedServices(compose)
	hasServiceAllowList := len(startedServices) > 0
	completedServices := environmentRestoreCompletedDependencyServices(compose, workspace)
	for _, raw := range checks {
		set.add(environmentRestoreApplyCompletedExpectation(raw, completedServices))
	}
	for _, component := range graph.Components {
		if !environmentRestoreShouldAddComponentHealth(component, startedServices, hasServiceAllowList) {
			continue
		}
		item, errText := environmentRestoreNormalizeComponentHealthCheck(component)
		if errText == "" {
			set.add(environmentRestoreApplyCompletedExpectation(item, completedServices))
		}
	}
	for _, service := range stringSliceFromAny(compose["services"]) {
		if set.covered[service] {
			continue
		}
		item := map[string]any{
			"id":      "compose-service-" + safeReportID(service),
			"kind":    "compose-service",
			"service": service,
		}
		set.add(environmentRestoreApplyCompletedExpectation(item, completedServices))
	}
	return set.out
}

func environmentRestoreRefreshCompletedExpectations(checks []any, compose map[string]any, workspace string) []any {
	completedServices := environmentRestoreCompletedDependencyServices(compose, workspace)
	if len(completedServices) == 0 {
		return checks
	}
	out := make([]any, 0, len(checks))
	for _, raw := range checks {
		out = append(out, environmentRestoreApplyCompletedExpectation(raw, completedServices))
	}
	return out
}

type environmentRestoreHealthCheckSet struct {
	out     []any
	covered map[string]bool
	seen    map[string]bool
}

func (s *environmentRestoreHealthCheckSet) add(raw any) {
	item, ok := raw.(map[string]any)
	if !ok {
		s.out = append(s.out, raw)
		return
	}
	if signature := environmentRestoreHealthCheckSignature(item); signature != "" {
		if s.seen[signature] {
			return
		}
		s.seen[signature] = true
	}
	if environmentRestoreHealthCheckCoversService(item) {
		if service := strings.TrimSpace(valueString(item["service"])); service != "" {
			s.covered[service] = true
		}
	}
	s.out = append(s.out, raw)
}

func environmentRestoreStartedServices(compose map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, service := range stringSliceFromAny(compose["services"]) {
		if service = strings.TrimSpace(service); service != "" {
			out[service] = true
		}
	}
	return out
}

func environmentRestoreShouldAddComponentHealth(component store.EnvironmentComponent, startedServices map[string]bool, hasServiceAllowList bool) bool {
	service := strings.TrimSpace(component.ComposeService)
	return !hasServiceAllowList || service == "" || startedServices[service]
}

func environmentRestoreHealthCheckCoversService(item map[string]any) bool {
	kind := strings.TrimSpace(valueString(item["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(item["type"]))
	}
	return kind == "compose-service" || kind == "url"
}

func environmentRestoreNormalizeComponentHealthCheck(component store.EnvironmentComponent) (map[string]any, string) {
	raw := strings.TrimSpace(component.HealthCheckJSON)
	normalized, errText := environmentRestoreDecodeHealthCheck(raw)
	if errText != "" {
		return nil, errText
	}
	environmentRestoreApplyComponentHealthDefaults(normalized, component)
	kind := environmentRestoreHealthCheckKind(normalized)
	normalized["kind"] = kind
	if environmentRestoreComponentRequiresURLHealth(component) && kind != "url" {
		return nil, strings.TrimSpace(component.Role) + " health check requires url"
	}
	if errText := environmentRestoreValidateHealthCheckKind(normalized, kind, component); errText != "" {
		return nil, errText
	}
	return normalized, ""
}

func environmentRestoreDecodeHealthCheck(raw string) (map[string]any, string) {
	if raw == "" || raw == "{}" {
		return nil, "missing health check"
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return nil, "invalid health check JSON: " + err.Error()
	}
	if len(item) == 0 {
		return nil, "missing health check"
	}
	normalized := map[string]any{}
	for key, value := range item {
		normalized[key] = value
	}
	return normalized, ""
}

func environmentRestoreApplyComponentHealthDefaults(normalized map[string]any, component store.EnvironmentComponent) {
	componentID := strings.TrimSpace(component.ComponentID)
	if strings.TrimSpace(valueString(normalized["id"])) == "" && componentID != "" {
		normalized["id"] = "component-" + safeReportID(componentID)
	}
	if componentID != "" {
		normalized["componentId"] = componentID
	}
	if strings.TrimSpace(valueString(normalized["service"])) == "" && strings.TrimSpace(component.ComposeService) != "" {
		normalized["service"] = strings.TrimSpace(component.ComposeService)
	}
}

func environmentRestoreHealthCheckKind(normalized map[string]any) string {
	kind := strings.TrimSpace(valueString(normalized["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(normalized["type"]))
	}
	if kind == "" && strings.TrimSpace(valueString(normalized["url"])) != "" {
		return "url"
	}
	return kind
}

func environmentRestoreValidateHealthCheckKind(normalized map[string]any, kind string, component store.EnvironmentComponent) string {
	switch kind {
	case "url":
		if strings.TrimSpace(valueString(normalized["url"])) == "" {
			return "url health check requires url"
		}
	case "tcp":
		if strings.TrimSpace(valueString(normalized["address"])) == "" {
			return "tcp health check requires address"
		}
	case "command":
		if strings.TrimSpace(valueString(normalized["command"])) == "" {
			return "command health check requires command"
		}
	case "compose-service":
		if strings.TrimSpace(valueString(normalized["service"])) == "" {
			normalized["service"] = strings.TrimSpace(component.ComposeService)
		}
		if strings.TrimSpace(valueString(normalized["service"])) == "" {
			return "compose-service health check requires service"
		}
	case "container":
		if strings.TrimSpace(valueString(normalized["container"])) == "" {
			return "container health check requires container"
		}
	default:
		if kind == "" {
			return "health check requires kind"
		}
		return "unsupported health check kind: " + kind
	}
	return ""
}

func environmentRestoreComponentRequiresURLHealth(component store.EnvironmentComponent) bool {
	role := strings.TrimSpace(strings.ToLower(component.Role))
	kind := strings.TrimSpace(strings.ToLower(component.Kind))
	return role == "business-service" || kind == "app"
}

func environmentRestoreHealthCheckSignature(item map[string]any) string {
	kind := strings.TrimSpace(valueString(item["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(item["type"]))
	}
	switch kind {
	case "url":
		return "url:" + strings.TrimSpace(valueString(item["url"]))
	case "tcp":
		return "tcp:" + strings.TrimSpace(valueString(item["address"]))
	case "command":
		return "command:" + strings.TrimSpace(valueString(item["command"]))
	case "compose-service":
		return "compose-service:" + strings.TrimSpace(valueString(item["service"]))
	case "container":
		return "container:" + strings.TrimSpace(valueString(item["container"]))
	default:
		return ""
	}
}

func environmentRestoreApplyCompletedExpectation(raw any, completedServices map[string]bool) any {
	item, ok := raw.(map[string]any)
	if !ok || len(completedServices) == 0 {
		return raw
	}
	kind := strings.TrimSpace(valueString(item["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(item["type"]))
	}
	service := strings.TrimSpace(valueString(item["service"]))
	if kind != "compose-service" || service == "" || !completedServices[service] {
		return raw
	}
	if strings.TrimSpace(valueString(item["expect"])) == "" {
		item["expect"] = "service_completed_successfully"
	}
	return item
}

func environmentRestoreCompletedDependencyServices(compose map[string]any, workspace string) map[string]bool {
	out := map[string]bool{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service := range parseComposeCompletedDependencyServices(content) {
			out[service] = true
		}
	}
	return out
}

func parseComposeCompletedDependencyServices(content string) map[string]bool {
	out := map[string]bool{}
	state := composeCompletedDependencyParseState{}
	for _, line := range strings.Split(content, "\n") {
		service := state.completedDependency(line)
		if service != "" {
			out[service] = true
		}
	}
	return out
}

type composeCompletedDependencyParseState struct {
	inServices        bool
	servicesIndent    int
	serviceIndent     int
	currentService    string
	inDependsOn       bool
	dependsIndent     int
	currentDependency string
	dependencyIndent  int
}

func (state *composeCompletedDependencyParseState) completedDependency(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	indent := leadingSpaceCount(line)
	if !state.inServices {
		if trimmed == composeServicesHeader {
			state.inServices = true
			state.servicesIndent = indent
			state.serviceIndent = -1
		}
		return ""
	}
	if indent <= state.servicesIndent {
		state.reset()
		return ""
	}
	if state.enterService(trimmed, indent) {
		return ""
	}
	if state.currentService == "" || indent <= state.serviceIndent {
		state.inDependsOn = false
		return ""
	}
	if strings.TrimSuffix(trimmed, ":") == "depends_on" {
		state.inDependsOn = true
		state.dependsIndent = indent
		state.currentDependency = ""
		state.dependencyIndent = -1
		return ""
	}
	if state.inDependsOn && indent <= state.dependsIndent {
		state.inDependsOn = false
		return ""
	}
	if !state.inDependsOn {
		return ""
	}
	return state.completedDependencyInDependsOn(trimmed, indent)
}

func (state *composeCompletedDependencyParseState) reset() {
	state.inServices = false
	state.currentService = ""
	state.inDependsOn = false
	state.currentDependency = ""
	state.serviceIndent = -1
	state.dependencyIndent = -1
}

func (state *composeCompletedDependencyParseState) enterService(trimmed string, indent int) bool {
	if strings.HasPrefix(trimmed, "-") || !strings.HasSuffix(trimmed, ":") {
		return false
	}
	if state.serviceIndent >= 0 && indent != state.serviceIndent {
		return false
	}
	state.serviceIndent = indent
	state.currentService = cleanComposeScalar(strings.TrimSpace(strings.TrimSuffix(trimmed, ":")))
	state.inDependsOn = false
	state.currentDependency = ""
	state.dependencyIndent = -1
	return true
}

func (state *composeCompletedDependencyParseState) completedDependencyInDependsOn(trimmed string, indent int) string {
	if strings.HasPrefix(trimmed, "-") {
		state.currentDependency = ""
		return ""
	}
	if dependency, condition, ok := composeFlowStyleDependencyCondition(trimmed); ok {
		state.currentDependency = ""
		if condition == "service_completed_successfully" {
			return dependency
		}
		return ""
	}
	if strings.HasSuffix(trimmed, ":") && (state.dependencyIndent < 0 || indent == state.dependencyIndent) {
		state.currentDependency = cleanComposeScalar(strings.TrimSpace(strings.TrimSuffix(trimmed, ":")))
		state.dependencyIndent = indent
		return ""
	}
	if state.currentDependency == "" || indent <= state.dependencyIndent || !strings.HasPrefix(trimmed, "condition:") {
		return ""
	}
	condition := cleanComposeScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "condition:")))
	if condition == "service_completed_successfully" {
		return state.currentDependency
	}
	return ""
}

func composeFlowStyleDependencyCondition(trimmed string) (string, string, bool) {
	dependency, rest, ok := strings.Cut(trimmed, ":")
	if !ok || !strings.Contains(rest, "condition") {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "{") || !strings.HasSuffix(rest, "}") {
		return "", "", false
	}
	fields := strings.Split(strings.Trim(rest, "{}"), ",")
	for _, field := range fields {
		key, value, ok := strings.Cut(field, ":")
		if !ok || cleanComposeScalar(key) != "condition" {
			continue
		}
		return cleanComposeScalar(dependency), cleanComposeScalar(value), true
	}
	return "", "", false
}
