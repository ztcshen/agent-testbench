package direct

import (
	"context"
	"fmt"
	"os"

	"agent-testbench/internal/store"
)

func BuildPlan(ctx context.Context, runtime store.Store, request RunRequest) (Plan, error) {
	registration, item, err := lookup(ctx, runtime, request.BindingID, request.CaseID)
	if err != nil {
		return Plan{}, err
	}
	plan := executionPlan(registration, item)
	plan.Overrides = publicFields(request.Overrides)
	if err := validateSource(registration.Registration); err != nil {
		return plan, err
	}
	return plan, nil
}

func executionPlan(item RegistrationResult, selected Case) Plan {
	registration := item.Registration
	return Plan{
		OK: true, DryRun: true,
		Mode: Mode, BindingID: registration.Binding.ID, CatalogRevision: item.Revision,
		ProfileID: item.ProfileID,
		Case:      selected, SourceDigest: registration.SourceDigest, SourceHashes: registration.SourceHashes,
		TargetURL: registration.Binding.TargetURL, Runtime: "native-process", Host: registration.Host,
		ManagesTarget: false, Executable: registration.Binding.Executable, WorkingDir: registration.Binding.ProjectDir,
	}
}

func validateSource(registration Registration) error {
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	if host != registration.Host {
		return fmt.Errorf("%w: direct binding belongs to execution host %q", ErrConflict, registration.Host)
	}
	hashes, err := sourceHashes(registration.Binding.ProjectDir, registration.Manifest)
	if err != nil {
		return fmt.Errorf("%w: registered source unavailable: %v", ErrConflict, err)
	}
	if sourceDigest(hashes) != registration.SourceDigest {
		return fmt.Errorf("%w: source changed; refresh the direct registration before a new execution", ErrConflict)
	}
	return nil
}
