package direct

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"agent-testbench/internal/runner/nativeprocess"
	"agent-testbench/internal/store"
)

func Register(ctx context.Context, runtime store.Store, request RegisterRequest) (RegistrationResult, error) {
	binding, err := normalizeBinding(request.Binding)
	if err != nil {
		return RegistrationResult{}, err
	}
	versioned, snapshot, err := currentCatalog(ctx, runtime)
	if err != nil {
		return RegistrationResult{}, err
	}
	if request.ExpectedRevision != nil && *request.ExpectedRevision != snapshot.Revision {
		return RegistrationResult{}, fmt.Errorf("%w: expected catalog revision %d, current %d", ErrConflict, *request.ExpectedRevision, snapshot.Revision)
	}
	manifest, err := discoverAdapter(ctx, binding)
	if err != nil {
		return RegistrationResult{}, err
	}
	hashes, err := sourceHashes(binding.ProjectDir, manifest)
	if err != nil {
		return RegistrationResult{}, err
	}
	host, err := os.Hostname()
	if err != nil {
		return RegistrationResult{}, err
	}
	registration := Registration{Mode: Mode, Binding: binding, Manifest: manifest, SourceHashes: hashes, SourceDigest: sourceDigest(hashes), Host: host}
	raw, err := json.Marshal(registration)
	if err != nil {
		return RegistrationResult{}, err
	}
	if len(raw) > 512<<10 {
		return RegistrationResult{}, fmt.Errorf("%w: direct registration exceeds 512 KiB", ErrInvalid)
	}
	entry := store.CatalogTemplateConfig{
		ID: configScope + ":" + binding.ID, ScopeType: configScope, ScopeID: binding.ID,
		Title: binding.DisplayName, ConfigJSON: string(raw), Status: "active",
	}
	replaceBinding(&snapshot.Catalog, entry)
	snapshot.Catalog.IndexedAt = time.Now().UTC()
	updated, err := versioned.CompareAndSwapProfileCatalog(ctx, snapshot.Revision, snapshot.Catalog, store.ProfileCatalogMutation{
		Operation: "direct.register", SummaryJSON: fmt.Sprintf(`{"bindingId":%q,"cases":%d}`, binding.ID, len(manifest.Cases)),
	})
	if err != nil {
		return RegistrationResult{}, err
	}
	return RegistrationResult{OK: true, Mode: Mode, ProfileID: updated.Catalog.ProfileID, Revision: updated.Revision, Registration: registration}, nil
}

func replaceBinding(catalog *store.ProfileCatalog, entry store.CatalogTemplateConfig) {
	for index := range catalog.TemplateConfigs {
		if catalog.TemplateConfigs[index].ID == entry.ID {
			catalog.TemplateConfigs[index] = entry
			return
		}
	}
	catalog.TemplateConfigs = append(catalog.TemplateConfigs, entry)
}

func currentCatalog(ctx context.Context, runtime store.Store) (store.VersionedProfileCatalogStore, store.ProfileCatalogSnapshot, error) {
	versioned, ok := runtime.(store.VersionedProfileCatalogStore)
	if !ok {
		return nil, store.ProfileCatalogSnapshot{}, fmt.Errorf("direct mode requires a versioned SQL Store")
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if errors.Is(err, store.ErrNotFound) || (err == nil && catalog.ProfileID == "") {
		return versioned, store.ProfileCatalogSnapshot{Catalog: store.ProfileCatalog{ProfileID: "default", IndexedAt: time.Now().UTC()}}, nil
	}
	if err != nil {
		return nil, store.ProfileCatalogSnapshot{}, err
	}
	snapshot, err := versioned.GetProfileCatalogSnapshot(ctx, catalog.ProfileID)
	return versioned, snapshot, err
}

func List(ctx context.Context, runtime store.Store) ([]RegistrationResult, error) {
	_, snapshot, err := currentCatalog(ctx, runtime)
	if err != nil {
		return nil, err
	}
	items := make([]RegistrationResult, 0)
	for _, config := range snapshot.Catalog.TemplateConfigs {
		if config.ScopeType != configScope || config.Status != "active" {
			continue
		}
		var registration Registration
		if err := json.Unmarshal([]byte(config.ConfigJSON), &registration); err != nil {
			return nil, err
		}
		if registration.Mode != Mode || registration.Binding.ID != config.ScopeID {
			return nil, fmt.Errorf("%w: inconsistent direct binding %q", ErrInvalid, config.ID)
		}
		items = append(items, RegistrationResult{OK: true, Mode: Mode, ProfileID: snapshot.Catalog.ProfileID, Revision: snapshot.Revision, Registration: registration})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Registration.Binding.ID < items[j].Registration.Binding.ID })
	return items, nil
}

func lookup(ctx context.Context, runtime store.Store, bindingID, caseID string) (RegistrationResult, Case, error) {
	items, err := List(ctx, runtime)
	if err != nil {
		return RegistrationResult{}, Case{}, err
	}
	for _, item := range items {
		if item.Registration.Binding.ID != bindingID {
			continue
		}
		for _, candidate := range item.Registration.Manifest.Cases {
			if candidate.ID == caseID {
				return item, candidate, nil
			}
		}
	}
	return RegistrationResult{}, Case{}, fmt.Errorf("%w: direct binding or case does not exist", store.ErrNotFound)
}

func discoverAdapter(ctx context.Context, binding Binding) (Manifest, error) {
	request := AdapterRequest{ProtocolVersion: ProtocolVersion, Operation: "discover", Mode: Mode}
	result := invoke(ctx, binding, request, 15*time.Second)
	if result.Category != "" {
		return Manifest{}, fmt.Errorf("adapter discovery %s: %s", result.Category, result.Error)
	}
	var manifest Manifest
	if err := decodeProtocol(result.Stdout, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func invoke(ctx context.Context, binding Binding, request AdapterRequest, timeout time.Duration) nativeprocess.Result {
	input, err := json.Marshal(request)
	if err != nil {
		return nativeprocess.Result{ExitCode: -1, Category: "protocol", Error: err.Error()}
	}
	return nativeprocess.Run(ctx, nativeprocess.Command{
		Executable: binding.Executable, Args: binding.Args, Directory: binding.ProjectDir,
		Env: adapterEnvironment(binding), Input: input, Timeout: timeout,
	})
}
