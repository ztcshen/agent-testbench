package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func environmentServices(services stringListFlag, repos stringListFlag, branches stringListFlag, repoRefs stringListFlag, checkouts stringListFlag) []map[string]any {
	repoByService := environmentKeyValueMap(repos)
	branchByService := environmentKeyValueMap(branches)
	refByService := environmentKeyValueMap(repoRefs)
	checkoutByService := environmentKeyValueMap(checkouts)
	ids := map[string]bool{}
	for _, id := range services.Values() {
		ids[id] = true
	}
	for id := range repoByService {
		ids[id] = true
	}
	for id := range branchByService {
		ids[id] = true
	}
	for id := range refByService {
		ids[id] = true
	}
	for id := range checkoutByService {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	out := make([]map[string]any, 0, len(ordered))
	for _, id := range ordered {
		item := map[string]any{"id": id}
		if repo := repoByService[id]; repo != "" {
			item["repo"] = repo
		}
		if branch := branchByService[id]; branch != "" {
			item["branch"] = branch
		}
		if ref := refByService[id]; ref != "" {
			item["ref"] = ref
		}
		if checkout := checkoutByService[id]; checkout != "" {
			item["checkout"] = checkout
		}
		out = append(out, item)
	}
	return out
}

func environmentRepoMap(repos stringListFlag, branches stringListFlag, repoRefs stringListFlag, checkouts stringListFlag) map[string]any {
	repoByService := environmentKeyValueMap(repos)
	branchByService := environmentKeyValueMap(branches)
	refByService := environmentKeyValueMap(repoRefs)
	checkoutByService := environmentKeyValueMap(checkouts)
	ids := map[string]bool{}
	for id := range repoByService {
		ids[id] = true
	}
	for id := range branchByService {
		ids[id] = true
	}
	for id := range refByService {
		ids[id] = true
	}
	for id := range checkoutByService {
		ids[id] = true
	}
	out := map[string]any{}
	for id := range ids {
		item := map[string]any{}
		if repo := repoByService[id]; repo != "" {
			item["url"] = repo
		}
		if branch := branchByService[id]; branch != "" {
			item["branch"] = branch
		}
		if ref := refByService[id]; ref != "" {
			item["ref"] = ref
		}
		if checkout := checkoutByService[id]; checkout != "" {
			item["checkout"] = checkout
		}
		out[id] = item
	}
	return out
}

func environmentComposeConfig(composeFiles stringListFlag, generatedFiles stringListFlag, startCommand string, projectName string, envFiles stringListFlag, envs stringListFlag, profiles stringListFlag, services stringListFlag, skipPull bool, skipBuild bool, packageRepo string, packageBranch string, packageRef string) (map[string]any, error) {
	files := composeFiles.Values()
	composeFile := ""
	if len(files) > 0 {
		composeFile = strings.TrimSpace(files[0])
	}
	out := map[string]any{
		"composeFile":  composeFile,
		"startCommand": strings.TrimSpace(startCommand),
	}
	if len(files) > 0 {
		out["composeFiles"] = files
	}
	generated, err := generatedFileContentMapFromFlags(generatedFiles)
	if err != nil {
		return nil, err
	}
	if len(generated) > 0 {
		out["generatedFiles"] = generated
	}
	if strings.TrimSpace(projectName) != "" {
		out["projectName"] = strings.TrimSpace(projectName)
	}
	if len(envFiles.Values()) > 0 {
		out["envFiles"] = envFiles.Values()
	}
	if values := keyValueMapFromFlags(envs); len(values) > 0 {
		out["env"] = values
	}
	if len(profiles.Values()) > 0 {
		out["profiles"] = profiles.Values()
	}
	if len(services.Values()) > 0 {
		out["services"] = services.Values()
	}
	if skipPull {
		out["skipPull"] = true
	}
	if skipBuild {
		out["skipBuild"] = true
	}
	packageConfig := map[string]string{}
	if strings.TrimSpace(packageRepo) != "" {
		packageConfig["url"] = strings.TrimSpace(packageRepo)
	}
	if strings.TrimSpace(packageBranch) != "" {
		packageConfig["branch"] = strings.TrimSpace(packageBranch)
	}
	if strings.TrimSpace(packageRef) != "" {
		packageConfig["ref"] = strings.TrimSpace(packageRef)
	}
	if len(packageConfig) > 0 {
		packageConfig["checkout"] = "."
		out["package"] = packageConfig
	}
	return out, nil
}

func generatedFileContentMapFromFlags(values stringListFlag) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range values.Values() {
		target, source, ok := strings.Cut(raw, "=")
		target = strings.TrimSpace(target)
		source = strings.TrimSpace(source)
		if !ok || target == "" || source == "" {
			return nil, fmt.Errorf("generated compose file must be TARGET=SOURCE_FILE, got %q", raw)
		}
		if filepath.IsAbs(target) || target == "." || target == ".." || strings.HasPrefix(filepath.Clean(target), ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("generated compose file target must be relative to the restore workspace: %s", target)
		}
		content, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read generated compose source %s: %w", source, err)
		}
		out[filepath.Clean(target)] = string(content)
	}
	return out, nil
}

func keyValueMapFromFlags(values stringListFlag) map[string]string {
	out := map[string]string{}
	for _, raw := range values.Values() {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func environmentHealthChecks(urls stringListFlag, tcpAddresses stringListFlag, commands stringListFlag, composeServices stringListFlag) []map[string]any {
	out := make([]map[string]any, 0, len(urls.Values())+len(tcpAddresses.Values())+len(commands.Values())+len(composeServices.Values()))
	index := 1
	for _, url := range urls.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "url", "url": url})
		index++
	}
	for _, address := range tcpAddresses.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "tcp", "address": address})
		index++
	}
	for _, command := range commands.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "command", "command": command})
		index++
	}
	for _, service := range composeServices.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "compose-service", "service": service})
		index++
	}
	return out
}

func environmentRepoUpdateMap(repos stringListFlag, branches stringListFlag, repoRefs stringListFlag, checkouts stringListFlag) map[string]map[string]string {
	repoByService := environmentKeyValueMap(repos)
	branchByService := environmentKeyValueMap(branches)
	refByService := environmentKeyValueMap(repoRefs)
	checkoutByService := environmentKeyValueMap(checkouts)
	updates := map[string]map[string]string{}
	add := func(serviceID, key, value string) {
		serviceID = strings.TrimSpace(serviceID)
		if serviceID == "" {
			return
		}
		if _, ok := updates[serviceID]; !ok {
			updates[serviceID] = map[string]string{}
		}
		updates[serviceID][key] = value
	}
	for serviceID, value := range repoByService {
		add(serviceID, "url", value)
	}
	for serviceID, value := range branchByService {
		add(serviceID, "branch", value)
	}
	for serviceID, value := range refByService {
		add(serviceID, "ref", value)
	}
	for serviceID, value := range checkoutByService {
		add(serviceID, "checkout", value)
	}
	return updates
}

func environmentServicesWithRepoUpdates(existing []any, updates map[string]map[string]string) []any {
	out := make([]any, 0, len(existing)+len(updates))
	seen := map[string]bool{}
	for _, raw := range existing {
		item := jsonObjectFromAny(raw)
		serviceID := strings.TrimSpace(valueString(item["id"]))
		if serviceID == "" {
			continue
		}
		if update, ok := updates[serviceID]; ok {
			applyEnvironmentServiceRepoUpdate(item, update)
		}
		seen[serviceID] = true
		out = append(out, item)
	}
	for _, serviceID := range sortedMapKeys(updates) {
		if seen[serviceID] {
			continue
		}
		item := map[string]any{"id": serviceID}
		applyEnvironmentServiceRepoUpdate(item, updates[serviceID])
		out = append(out, item)
	}
	return out
}

func environmentKeyValueMap(values stringListFlag) map[string]string {
	out := map[string]string{}
	for _, value := range values.Values() {
		key, raw, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		raw = strings.TrimSpace(raw)
		if !ok || key == "" || raw == "" {
			continue
		}
		out[key] = raw
	}
	return out
}
