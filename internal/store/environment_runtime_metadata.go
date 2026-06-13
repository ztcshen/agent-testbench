package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

func MergeEnvironmentRuntimeMetadataIntoJSON(env Environment, services []EnvironmentService, checks []EnvironmentHealthCheck) (Environment, error) {
	env = EnvironmentWithoutStructuredRuntimeMetadata(env, services, checks)
	env, err := mergeEnvironmentServicesIntoJSON(env, services)
	if err != nil {
		return Environment{}, err
	}
	return mergeEnvironmentHealthChecksIntoJSON(env, checks)
}

func EnvironmentWithoutStructuredRuntimeMetadata(env Environment, services []EnvironmentService, checks []EnvironmentHealthCheck) Environment {
	if len(services) > 0 {
		serviceIDs := environmentServiceIDSet(services)
		env.ServicesJSON = mustJSON(environmentServiceJSONItemsWithoutIDs(env.ServicesJSON, serviceIDs), "[]")
		env.ReposJSON = mustJSON(environmentRepoJSONItemsWithoutIDs(env.ReposJSON, serviceIDs), "{}")
	}
	if len(checks) > 0 {
		checkIDs := environmentHealthCheckIDSet(checks)
		signatures := environmentHealthCheckSignatureSet(checks)
		env.HealthChecksJSON = mustJSON(environmentHealthCheckJSONItemsWithoutMatches(env.HealthChecksJSON, checkIDs, signatures), "[]")
	}
	return env
}

func mergeEnvironmentServicesIntoJSON(env Environment, services []EnvironmentService) (Environment, error) {
	services = NormalizeEnvironmentServices(services)
	if len(services) == 0 {
		return env, nil
	}
	serviceIDs := environmentServiceIDSet(services)
	serviceItems := environmentServiceJSONItemsWithoutIDs(env.ServicesJSON, serviceIDs)
	repoItems := environmentRepoJSONItemsWithoutIDs(env.ReposJSON, serviceIDs)
	for _, service := range services {
		item := map[string]any{"id": service.ServiceID}
		repo := map[string]any{}
		if service.RepoURL != "" {
			item["repo"] = service.RepoURL
			repo["url"] = service.RepoURL
		}
		if service.Branch != "" {
			item["branch"] = service.Branch
			repo["branch"] = service.Branch
		}
		if service.Ref != "" {
			item["ref"] = service.Ref
			repo["ref"] = service.Ref
		}
		if service.Checkout != "" {
			item["checkout"] = service.Checkout
			repo["checkout"] = service.Checkout
		}
		serviceItems = append(serviceItems, item)
		if len(repo) > 0 {
			repoItems[service.ServiceID] = repo
		}
	}
	servicesJSON, err := json.Marshal(serviceItems)
	if err != nil {
		return Environment{}, fmt.Errorf("encode structured environment services: %w", err)
	}
	reposJSON, err := json.Marshal(repoItems)
	if err != nil {
		return Environment{}, fmt.Errorf("encode structured environment repositories: %w", err)
	}
	env.ServicesJSON = string(servicesJSON)
	env.ReposJSON = string(reposJSON)
	return env, nil
}

func mergeEnvironmentHealthChecksIntoJSON(env Environment, checks []EnvironmentHealthCheck) (Environment, error) {
	checks = NormalizeEnvironmentHealthChecks(checks)
	if len(checks) == 0 {
		return env, nil
	}
	checkIDs := environmentHealthCheckIDSet(checks)
	signatures := environmentHealthCheckSignatureSet(checks)
	items := environmentHealthCheckJSONItemsWithoutMatches(env.HealthChecksJSON, checkIDs, signatures)
	for _, check := range checks {
		item := map[string]any{"id": check.CheckID, "kind": check.Kind}
		if check.URL != "" {
			item["url"] = check.URL
		}
		if check.Address != "" {
			item["address"] = check.Address
		}
		if check.Command != "" {
			item["command"] = check.Command
		}
		if check.ComposeService != "" {
			item["service"] = check.ComposeService
		}
		if check.Expect != "" {
			item["expect"] = check.Expect
		}
		items = append(items, item)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return Environment{}, fmt.Errorf("encode structured environment health checks: %w", err)
	}
	env.HealthChecksJSON = string(raw)
	return env, nil
}

func environmentServiceIDSet(services []EnvironmentService) map[string]bool {
	out := map[string]bool{}
	for _, service := range services {
		if id := strings.TrimSpace(service.ServiceID); id != "" {
			out[id] = true
		}
	}
	return out
}

func environmentHealthCheckIDSet(checks []EnvironmentHealthCheck) map[string]bool {
	out := map[string]bool{}
	for _, check := range checks {
		if id := strings.TrimSpace(check.CheckID); id != "" {
			out[id] = true
		}
	}
	return out
}

func environmentHealthCheckSignatureSet(checks []EnvironmentHealthCheck) map[string]bool {
	out := map[string]bool{}
	for _, check := range checks {
		if signature := environmentHealthCheckSignatureFromCheck(check); signature != "" {
			out[signature] = true
		}
	}
	return out
}

func environmentServiceJSONItemsWithoutIDs(raw string, ids map[string]bool) []map[string]any {
	return environmentJSONArrayItemsWithoutIDs(raw, ids)
}

func environmentRepoJSONItemsWithoutIDs(raw string, ids map[string]bool) map[string]any {
	var repos map[string]any
	if err := json.Unmarshal([]byte(environmentJSONDefault(raw, "{}")), &repos); err != nil || repos == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for id, repo := range repos {
		if ids[strings.TrimSpace(id)] {
			continue
		}
		out[id] = repo
	}
	return out
}

func environmentHealthCheckJSONItemsWithoutMatches(raw string, ids map[string]bool, signatures map[string]bool) []map[string]any {
	items := environmentJSONArrayItemsWithoutIDs(raw, ids)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if signatures[environmentHealthCheckSignatureFromJSON(item)] {
			continue
		}
		out = append(out, item)
	}
	return out
}

func environmentJSONArrayItemsWithoutIDs(raw string, ids map[string]bool) []map[string]any {
	var items []map[string]any
	if err := json.Unmarshal([]byte(environmentJSONDefault(raw, "[]")), &items); err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if ids[strings.TrimSpace(valueStringFromAny(item["id"]))] {
			continue
		}
		out = append(out, item)
	}
	return out
}

func environmentHealthCheckSignatureFromCheck(check EnvironmentHealthCheck) string {
	return environmentHealthCheckSignature(
		check.Kind,
		check.URL,
		check.Address,
		check.Command,
		check.ComposeService,
		check.Expect,
	)
}

func environmentHealthCheckSignatureFromJSON(item map[string]any) string {
	kind := strings.TrimSpace(valueStringFromAny(item["kind"]))
	if kind == "" {
		kind = environmentHealthCheckKindFromLegacyJSON(item)
	}
	return environmentHealthCheckSignature(
		kind,
		valueStringFromAny(item["url"]),
		valueStringFromAny(item["address"]),
		valueStringFromAny(item["command"]),
		valueStringFromAny(item["service"]),
		valueStringFromAny(item["expect"]),
	)
}

func environmentHealthCheckSignature(kind string, url string, address string, command string, service string, expect string) string {
	parts := []string{
		strings.TrimSpace(kind),
		strings.TrimSpace(url),
		strings.TrimSpace(address),
		strings.TrimSpace(command),
		strings.TrimSpace(service),
		strings.TrimSpace(expect),
	}
	if parts[1] == "" && parts[2] == "" && parts[3] == "" && parts[4] == "" {
		return ""
	}
	return strings.Join(parts, "\x00")
}

func environmentJSONDefault(raw string, defaultValue string) string {
	if strings.TrimSpace(raw) == "" {
		return defaultValue
	}
	return raw
}

func mustJSON(value any, fallback string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(raw)
}
