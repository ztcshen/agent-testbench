// Package profileverify verifies Store state after profile publication.
package profileverify

import (
	"context"
	"errors"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

type PublishedProfile struct {
	ProfileID       string
	BundleDigest    string
	ConfigVersionID string
}

type Options struct {
	RequireCaseRuns     bool
	RequireWorkflowRuns bool
	ReadModelKeys       []string
}

type Summary struct {
	TotalChecks          int    `json:"totalChecks"`
	PassedChecks         int    `json:"passedChecks"`
	FailedChecks         int    `json:"failedChecks"`
	RequiredCaseRuns     bool   `json:"requiredCaseRuns"`
	RequiredWorkflowRuns bool   `json:"requiredWorkflowRuns"`
	FirstFailed          string `json:"firstFailed,omitempty"`
}

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func PublishedChecks(ctx context.Context, runtime store.Store, bundle profile.Bundle, published PublishedProfile, options Options) ([]Check, error) {
	checks := make([]Check, 0, 6)
	index, err := runtime.GetProfileIndex(ctx, published.ProfileID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			checks = appendCheck(checks, "profile-index", false, "profile index was not written")
			return checks, nil
		}
		return nil, err
	}
	checks = appendCheck(checks, "profile-index", index.BundleDigest == published.BundleDigest, "profile index digest matches published bundle")

	catalogIndex, err := runtime.GetProfileCatalogIndex(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			checks = appendCheck(checks, "catalog-index", false, "catalog index was not written")
		} else {
			return nil, err
		}
	} else {
		checks = appendCheck(checks, "catalog-index", catalogIndex.ProfileID == published.ProfileID, "catalog index points to active profile")
	}

	activeConfig, err := runtime.GetActiveConfigVersion(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			checks = appendCheck(checks, "active-config", false, "active config version was not written")
		} else {
			return nil, err
		}
	} else {
		ok := activeConfig.ID == published.ConfigVersionID &&
			activeConfig.ProfileID == published.ProfileID &&
			activeConfig.BundleDigest == published.BundleDigest
		checks = appendCheck(checks, "active-config", ok, "active config version matches published bundle")
	}

	readModelChecks, err := readModelChecks(ctx, runtime, published, options.ReadModelKeys)
	if err != nil {
		return nil, err
	}
	checks = append(checks, readModelChecks...)

	if options.RequireCaseRuns {
		caseRunChecks, err := apiCaseRunChecks(ctx, runtime, bundle)
		if err != nil {
			return nil, err
		}
		checks = append(checks, caseRunChecks...)
	}
	if options.RequireWorkflowRuns {
		workflowChecks, err := workflowRunChecks(ctx, runtime, bundle)
		if err != nil {
			return nil, err
		}
		checks = append(checks, workflowChecks...)
	}
	return checks, nil
}

func Summarize(checks []Check, options Options) Summary {
	summary := Summary{
		TotalChecks:          len(checks),
		RequiredCaseRuns:     options.RequireCaseRuns,
		RequiredWorkflowRuns: options.RequireWorkflowRuns,
	}
	for _, check := range checks {
		if check.OK {
			summary.PassedChecks++
			continue
		}
		summary.FailedChecks++
		if summary.FirstFailed == "" {
			summary.FirstFailed = check.Name
		}
	}
	return summary
}

func ChecksOK(checks []Check) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func FirstFailed(checks []Check) string {
	for _, check := range checks {
		if !check.OK {
			return check.Name + ": " + check.Detail
		}
	}
	return "no checks passed"
}

func readModelChecks(ctx context.Context, runtime store.Store, published PublishedProfile, keys []string) ([]Check, error) {
	checks := make([]Check, 0, len(keys))
	for _, key := range keys {
		model, err := runtime.GetReadModel(ctx, published.ProfileID, key)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				checks = appendCheck(checks, "read-model:"+key, false, "read model was not written")
				continue
			}
			return nil, err
		}
		ok := model.ConfigVersionID == published.ConfigVersionID && strings.TrimSpace(model.PayloadJSON) != ""
		checks = appendCheck(checks, "read-model:"+key, ok, "read model exists for published config version")
	}
	return checks, nil
}

func workflowRunChecks(ctx context.Context, runtime store.Store, bundle profile.Bundle) ([]Check, error) {
	if len(bundle.Workflows) == 0 {
		return []Check{{Name: "workflow-runs", OK: true, Detail: "profile declares no workflows"}}, nil
	}
	runs, err := runtime.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	latestByWorkflow := map[string]store.Run{}
	for _, item := range runs {
		if item.WorkflowID == "" {
			continue
		}
		current, ok := latestByWorkflow[item.WorkflowID]
		if !ok || item.CreatedAt.After(current.CreatedAt) || (item.CreatedAt.Equal(current.CreatedAt) && item.ID > current.ID) {
			latestByWorkflow[item.WorkflowID] = item
		}
	}
	checks := make([]Check, 0, len(bundle.Workflows))
	for _, item := range bundle.Workflows {
		run, ok := latestByWorkflow[item.ID]
		if !ok || !isPassedStatus(run.Status) {
			checks = appendCheck(checks, "workflow-run:"+item.ID, false, "no passed run recorded in Store")
			continue
		}
		checks = appendCheck(checks, "workflow-run:"+item.ID, true, "latest Workflow run passed")
	}
	return checks, nil
}

func apiCaseRunChecks(ctx context.Context, runtime store.Store, bundle profile.Bundle) ([]Check, error) {
	if len(bundle.APICases) == 0 {
		return []Check{{Name: "api-case-runs", OK: true, Detail: "profile declares no API cases"}}, nil
	}
	latestStore, ok := runtime.(interface {
		ListLatestAPICaseRuns(context.Context) ([]store.APICaseRun, error)
	})
	if !ok {
		return nil, errors.New("runtime store does not support latest API case run lookup")
	}
	latestRuns, err := latestStore.ListLatestAPICaseRuns(ctx)
	if err != nil {
		return nil, err
	}
	latestByCase := map[string]store.APICaseRun{}
	for _, item := range latestRuns {
		latestByCase[item.CaseID] = item
	}
	checks := make([]Check, 0, len(bundle.APICases))
	for _, item := range bundle.APICases {
		run, ok := latestByCase[item.ID]
		if !ok || !isPassedStatus(run.Status) {
			checks = appendCheck(checks, "api-case-run:"+item.ID, false, "no passed run recorded in Store")
			continue
		}
		checks = appendCheck(checks, "api-case-run:"+item.ID, true, "latest API case run passed")
	}
	return checks, nil
}

func isPassedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success", "ok":
		return true
	default:
		return false
	}
}

func appendCheck(checks []Check, name string, ok bool, detail string) []Check {
	return append(checks, Check{Name: name, OK: ok, Detail: detail})
}
