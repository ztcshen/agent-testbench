package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func loadInterfaceNodeReportBundle(ctx context.Context, profileRef string, profileHomeRef string, storeURL string) (profile.Bundle, store.Store, func(), error) {
	cleanup := func() {}
	var sourceStore store.Store
	if strings.TrimSpace(profileRef) != "" {
		resolvedProfilePath, err := resolveProfileReference(profileRef, profileHomeRef)
		if err != nil {
			cleanup()
			return profile.Bundle{}, nil, func() {}, err
		}
		if strings.TrimSpace(storeURL) != "" {
			if err := guardProfilePublishTarget(resolvedProfilePath, storeURL); err != nil {
				return profile.Bundle{}, nil, cleanup, err
			}
			opened, err := openStore(ctx, storeURL)
			if err != nil {
				return profile.Bundle{}, nil, cleanup, err
			}
			sourceStore = opened
			cleanup = cleanupCLIStore(opened)
		}
		bundle, err := profile.Load(resolvedProfilePath)
		if err != nil {
			cleanup()
			return profile.Bundle{}, nil, func() {}, err
		}
		return bundle, sourceStore, cleanup, nil
	}
	if strings.TrimSpace(storeURL) != "" {
		opened, err := openStore(ctx, storeURL)
		if err != nil {
			return profile.Bundle{}, nil, cleanup, err
		}
		sourceStore = opened
		cleanup = cleanupCLIStore(opened)
	}
	if sourceStore == nil {
		return profile.Bundle{}, nil, cleanup, errors.New("--profile, --store, --store-url, or an active Store is required")
	}
	bundle, err := serveBundle(ctx, sourceStore)
	if err != nil {
		cleanup()
		return profile.Bundle{}, nil, func() {}, err
	}
	return bundle, sourceStore, cleanup, nil
}

func loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx context.Context, profileRef string, profileHomeRef string, storeRef string, legacyStoreURL string) (profile.Bundle, store.Store, string, func(), error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, legacyStoreURL)
	if err != nil {
		return profile.Bundle{}, nil, "", func() {}, err
	}
	bundle, runtime, cleanup, err := loadInterfaceNodeReportBundle(ctx, profileRef, profileHomeRef, resolvedStoreURL)
	if err != nil {
		return profile.Bundle{}, nil, resolvedStoreURL, cleanup, err
	}
	return bundle, runtime, resolvedStoreURL, cleanup, nil
}

func resolveDiscoveryInputs(profileRef string, storeRef string, legacyStoreURL string, offlineTemplatePackage bool) (string, string, error) {
	profileRef = strings.TrimSpace(profileRef)
	storeRef = strings.TrimSpace(storeRef)
	legacyStoreURL = strings.TrimSpace(legacyStoreURL)
	if offlineTemplatePackage {
		if profileRef == "" {
			return "", "", errors.New("--offline-template-package requires --profile")
		}
		if storeRef != "" || legacyStoreURL != "" {
			return "", "", errors.New("--offline-template-package cannot be combined with --store or --store-url")
		}
		return profileRef, "", nil
	}
	if profileRef != "" {
		return "", "", errors.New("--profile is for offline template package review; add --offline-template-package or use --store NAME_OR_DSN")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, legacyStoreURL)
	if err != nil {
		return "", "", err
	}
	return "", resolvedStoreURL, nil
}

func findInterfaceNodeByID(nodes []profile.InterfaceNode, id string) (profile.InterfaceNode, error) {
	id = strings.TrimSpace(id)
	for _, node := range nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return profile.InterfaceNode{}, fmt.Errorf("interface node not found: %s", id)
}

func normalizedDiscoveryText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "interface")
	value = strings.TrimSuffix(value, "api")
	value = strings.TrimSuffix(value, "接口")
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "/", "")
	return replacer.Replace(strings.TrimSpace(value))
}

func matchesDiscoveryFilter(filter string, values ...string) bool {
	needle := normalizedDiscoveryText(filter)
	if needle == "" {
		return true
	}
	for _, value := range values {
		haystack := normalizedDiscoveryText(value)
		if haystack != "" && (strings.Contains(haystack, needle) || strings.Contains(needle, haystack)) {
			return true
		}
	}
	return false
}
