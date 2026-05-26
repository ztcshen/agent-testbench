package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	storeopen "agent-testbench/internal/store/open"
)

func runServe(args []string) error {
	cfg, err := serveConfigFromArgs(args)
	if err != nil {
		return err
	}
	handler, cleanup, err := serveHandler(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := cleanup(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close serve store: %v\n", closeErr)
		}
	}()

	addr := cfg.host + ":" + strconv.Itoa(cfg.port)
	fmt.Printf("AgentTestBench listening on http://%s\n", addr)
	return http.ListenAndServe(addr, handler)
}

type serveConfig struct {
	profilePath     string
	profileHome     string
	host            string
	port            int
	storeRef        string
	storeURL        string
	traceGraphQLURL string
}

func serveHandlerFromArgs(args []string) (http.Handler, func() error, error) {
	cfg, err := serveConfigFromArgs(args)
	if err != nil {
		return nil, nil, err
	}
	return serveHandler(cfg)
}

func serveConfigFromArgs(args []string) (serveConfig, error) {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	host := flags.String("host", "127.0.0.1", "HTTP host")
	port := flags.Int("port", 18191, "HTTP port")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	traceGraphQLURL := flags.String("trace-graphql-url", os.Getenv("AGENT_TESTBENCH_TRACE_GRAPHQL_URL"), "Trace provider GraphQL URL")
	if err := flags.Parse(args); err != nil {
		return serveConfig{}, err
	}
	return serveConfig{profilePath: *profilePath, profileHome: *profileHome, host: *host, port: *port, storeRef: *storeRef, storeURL: *storeURL, traceGraphQLURL: *traceGraphQLURL}, nil
}

func serveHandler(cfg serveConfig) (http.Handler, func() error, error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(cfg.storeRef, cfg.storeURL)
	if err != nil {
		return nil, nil, err
	}
	storeLabel := resolvedStoreURL
	storeInfo := serveStoreInfo(cfg, resolvedStoreURL)
	runtime, err := storeopen.Open(context.Background(), resolvedStoreURL)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	if strings.TrimSpace(cfg.profilePath) != "" {
		profilePath, err := resolveProfileReference(cfg.profilePath, cfg.profileHome)
		if err != nil {
			return nil, nil, closeServeRuntime(runtime, err)
		}
		if _, err := publishProfileBundleToStore(ctx, runtime, profilePath, storeLabel, false, false); err != nil {
			return nil, nil, closeServeRuntime(runtime, err)
		}
	}
	bundle, err := serveBundle(ctx, runtime)
	if err != nil {
		return nil, nil, closeServeRuntime(runtime, err)
	}
	return controlplane.NewWithOptions(bundle, controlplane.Options{Runtime: runtime, TraceGraphQLURL: cfg.traceGraphQLURL, ProfileHome: cfg.profileHome, StoreInfo: storeInfo}), runtime.Close, nil
}

func closeServeRuntime(runtime store.Store, primaryErr error) error {
	if runtime == nil {
		return primaryErr
	}
	if closeErr := runtime.Close(); closeErr != nil {
		return fmt.Errorf("%w; close store: %v", primaryErr, closeErr)
	}
	return primaryErr
}

func serveBundle(ctx context.Context, runtime store.Store) (profile.Bundle, error) {
	if runtime != nil {
		catalog, err := runtime.GetProfileCatalog(ctx)
		if err == nil && catalog.ProfileID != "" {
			return profilecatalog.ToBundle(catalog), nil
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return profile.Bundle{}, err
		}
		if catalogIndex, err := runtime.GetProfileCatalogIndex(ctx); err == nil && strings.TrimSpace(catalogIndex.ProfileID) != "" {
			if profileIndex, err := runtime.GetProfileIndex(ctx, catalogIndex.ProfileID); err == nil && strings.TrimSpace(profileIndex.BundlePath) != "" {
				if bundle, err := profile.Load(profileIndex.BundlePath); err == nil {
					return bundle, nil
				}
			}
		}
	}
	return profile.EmptyBundle(), nil
}

func serveStoreInfo(cfg serveConfig, resolvedStoreURL string) controlplane.StoreInfo {
	backend, err := storeBackendFromURL(resolvedStoreURL)
	if err != nil {
		backend = ""
	}
	info := controlplane.StoreInfo{
		Configured: true,
		Backend:    backend,
		URL:        maskStoreURL(resolvedStoreURL),
		Source:     "active-config",
	}
	if strings.TrimSpace(cfg.storeURL) != "" {
		info.Source = "store-url"
		return info
	}
	storeRef := strings.TrimSpace(cfg.storeRef)
	if storeRef == "" {
		if entry, err := activeStoreConfig(); err == nil {
			info.Name = entry.Name
		}
		return info
	}
	if directBackend, err := storeBackendFromURL(storeRef); err == nil && directBackend != "" {
		info.Source = "store-flag"
		return info
	}
	info.Source = "store-config"
	info.Name = storeRef
	return info
}
