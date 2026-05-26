package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profileaudit"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/domain/profilehome"
	"agent-testbench/internal/profilepublish"
	"agent-testbench/internal/profileverify"
	"agent-testbench/internal/store"
)

type profileImportRequest struct {
	TemplatePackagePath string `json:"templatePackagePath"`
	Path                string `json:"path"`
	Audit               bool   `json:"audit"`
	RequireAuditOK      bool   `json:"requireAuditOk"`
	RequireCaseRuns     bool   `json:"requireCaseRuns"`
	RequireWorkflowRuns bool   `json:"requireWorkflowRuns"`
	Force               bool   `json:"force"`
}

type profileInstallRequest struct {
	TemplatePackagePath string `json:"templatePackagePath"`
	Path                string `json:"path"`
	Force               bool   `json:"force"`
}

type profileAuditPlanRequest struct {
	TemplatePackagePath string `json:"templatePackagePath"`
	Path                string `json:"path"`
	Force               bool   `json:"force"`
}

type profileImportResponse struct {
	TemplatePackageID     string                     `json:"templatePackageId"`
	TemplatePackagePath   string                     `json:"templatePackagePath"`
	TemplatePackageDigest string                     `json:"templatePackageDigest"`
	ProfileID             string                     `json:"profileId"`
	BundlePath            string                     `json:"bundlePath"`
	BundleDigest          string                     `json:"bundleDigest"`
	ImportedAt            time.Time                  `json:"importedAt"`
	Counts                profileImportCounts        `json:"counts"`
	Store                 profileImportStore         `json:"store"`
	ConfigVersion         profileImportConfigVersion `json:"configVersion"`
	ReadModels            []string                   `json:"readModels"`
	Audit                 *profileaudit.Report       `json:"audit,omitempty"`
}

type profileImportCounts struct {
	Services         int `json:"services"`
	Workflows        int `json:"workflows"`
	InterfaceNodes   int `json:"interfaceNodes"`
	APICases         int `json:"apiCases"`
	RequestTemplates int `json:"requestTemplates"`
	CaseDependencies int `json:"caseDependencies"`
	WorkflowBindings int `json:"workflowBindings"`
	Fixtures         int `json:"fixtures"`
}

type profileImportStore struct {
	TemplatePackageID     string    `json:"templatePackageId"`
	TemplatePackagePath   string    `json:"templatePackagePath"`
	TemplatePackageDigest string    `json:"templatePackageDigest"`
	ProfileID             string    `json:"profileId"`
	BundlePath            string    `json:"bundlePath"`
	BundleDigest          string    `json:"bundleDigest"`
	ImportedAt            time.Time `json:"importedAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type profileImportConfigVersion struct {
	ID                    string    `json:"id"`
	TemplatePackageID     string    `json:"templatePackageId"`
	TemplatePackagePath   string    `json:"templatePackagePath"`
	TemplatePackageDigest string    `json:"templatePackageDigest"`
	ProfileID             string    `json:"profileId"`
	SourcePath            string    `json:"sourcePath"`
	BundleDigest          string    `json:"bundleDigest"`
	Active                bool      `json:"active"`
	PublishedAt           time.Time `json:"publishedAt"`
	CreatedAt             time.Time `json:"createdAt"`
}

type profileVerifyResponse struct {
	OK                bool                  `json:"ok"`
	Error             string                `json:"error,omitempty"`
	TemplatePackageID string                `json:"templatePackageId"`
	ProfileID         string                `json:"profileId"`
	Audit             profileaudit.Report   `json:"audit"`
	Publish           profileImportResponse `json:"publish"`
	Summary           profileVerifySummary  `json:"summary"`
	Checks            []profileVerifyCheck  `json:"checks"`
}

type profileVerifySummary = profileverify.Summary
type profileVerifyCheck = profileverify.Check
type profileVerifyOptions = profileverify.Options

func templatePackageRequestPath(templatePackagePath string, legacyPath string) string {
	if value := strings.TrimSpace(templatePackagePath); value != "" {
		return value
	}
	return strings.TrimSpace(legacyPath)
}

func handleProfileImport(w http.ResponseWriter, r *http.Request, runtime store.Store, activate func(profile.Bundle), profileHome string) {
	if runtime == nil {
		writeJSONStatus(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "runtime store is not configured"})
		return
	}
	var req profileImportRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	req.Path = templatePackageRequestPath(req.TemplatePackagePath, req.Path)
	if req.Path == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "path is required"})
		return
	}
	resolvedPath, err := profilehome.ResolveReference(req.Path, profileHome)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	req.Path, err = materializeImportProfilePath(resolvedPath, profileHome, req.Force)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	bundle, report, err := importProfileBundle(r.Context(), runtime, req)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.HasPrefix(err.Error(), "load profile") || strings.HasPrefix(err.Error(), "digest profile") || strings.HasPrefix(err.Error(), "audit profile") || strings.HasPrefix(err.Error(), "profile audit failed") {
			status = http.StatusBadRequest
		}
		writeJSONStatus(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if activate != nil {
		activate(bundle)
	}
	writeJSON(w, report)
}

func handleProfileVerify(w http.ResponseWriter, r *http.Request, runtime store.Store, activate func(profile.Bundle), profileHome string) {
	if runtime == nil {
		writeJSONStatus(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "runtime store is not configured"})
		return
	}
	var req profileImportRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	req.Path = templatePackageRequestPath(req.TemplatePackagePath, req.Path)
	if req.Path == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "path is required"})
		return
	}
	resolvedPath, err := profilehome.ResolveReference(req.Path, profileHome)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	req.Path, err = materializeImportProfilePath(resolvedPath, profileHome, req.Force)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	bundle, report, err := verifyProfileBundle(r.Context(), runtime, req.Path, profileVerifyOptions{
		RequireCaseRuns:     req.RequireCaseRuns,
		RequireWorkflowRuns: req.RequireWorkflowRuns,
	})
	if err != nil {
		if report.ProfileID != "" {
			if report.Error == "" {
				report.Error = err.Error()
			}
			writeJSONStatus(w, http.StatusBadRequest, report)
			return
		}
		status := http.StatusInternalServerError
		if isProfileRequestError(err) {
			status = http.StatusBadRequest
		}
		writeJSONStatus(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if activate != nil {
		activate(bundle)
	}
	writeJSON(w, report)
}

func handleProfileAuditPlan(w http.ResponseWriter, r *http.Request, runtime store.Store, profileHome string) {
	var req profileAuditPlanRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	req.Path = templatePackageRequestPath(req.TemplatePackagePath, req.Path)
	if req.Path == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "path is required"})
		return
	}
	resolvedPath, err := profilehome.ResolveReference(req.Path, profileHome)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	resolvedPath, err = materializeImportProfilePath(resolvedPath, profileHome, req.Force)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	bundle, err := profile.Load(resolvedPath)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	audit, err := profileaudit.Audit(r.Context(), profileaudit.Options{
		Bundle:     bundle,
		BundlePath: resolvedPath,
		Store:      runtime,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, profileaudit.RepairPlan(audit))
}

func materializeImportProfilePath(path string, profileHome string, force bool) (string, error) {
	if !profilehome.IsArchivePath(path) {
		return path, nil
	}
	report, err := profilehome.Install(path, profileHome, force)
	if err != nil {
		return "", err
	}
	return report.TargetPath, nil
}

func handleInstalledProfiles(w http.ResponseWriter, _ *http.Request, profileHome string) {
	report, err := profilehome.List(profileHome)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, report)
}

func handleProfileInstall(w http.ResponseWriter, r *http.Request, profileHome string) {
	var req profileInstallRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	req.Path = templatePackageRequestPath(req.TemplatePackagePath, req.Path)
	if req.Path == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "path is required"})
		return
	}
	report, err := profilehome.Install(req.Path, profileHome, req.Force)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, report)
}

func importProfileBundle(ctx context.Context, runtime store.Store, req profileImportRequest) (profile.Bundle, profileImportResponse, error) {
	result, err := profilepublish.Publish(ctx, runtime, profilepublish.Options{
		Path:             req.Path,
		RequireAuditOK:   req.RequireAuditOK,
		UpsertReadModels: UpsertProfileReadModels,
	})
	if err != nil {
		return profile.Bundle{}, profileImportResponse{}, err
	}
	response := profileImportResponse{
		TemplatePackageID:     result.Bundle.ID,
		TemplatePackagePath:   req.Path,
		TemplatePackageDigest: result.Digest,
		ProfileID:             result.Bundle.ID,
		BundlePath:            req.Path,
		BundleDigest:          result.Digest,
		ImportedAt:            result.ImportedAt,
		Counts:                profileImportCountsFrom(result.Counts),
		Store:                 profileImportStoreFromIndex(result.Index),
		ConfigVersion:         profileImportConfigVersionFromStore(result.ConfigVersion),
		ReadModels:            result.ReadModels,
	}
	if req.Audit {
		auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
			Bundle:     result.Bundle,
			BundlePath: req.Path,
			Store:      runtime,
		})
		if err != nil {
			return profile.Bundle{}, profileImportResponse{}, fmt.Errorf("audit profile %q: %w", result.Bundle.ID, err)
		}
		response.Audit = &auditReport
	}
	return result.Bundle, response, nil
}

func verifyProfileBundle(ctx context.Context, runtime store.Store, path string, options profileVerifyOptions) (profile.Bundle, profileVerifyResponse, error) {
	bundle, err := profile.Load(path)
	if err != nil {
		return profile.Bundle{}, profileVerifyResponse{}, fmt.Errorf("load profile %q: %w", path, err)
	}
	auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
		Bundle:     bundle,
		BundlePath: path,
	})
	if err != nil {
		return profile.Bundle{}, profileVerifyResponse{}, fmt.Errorf("audit profile %q: %w", bundle.ID, err)
	}
	if !auditReport.OK {
		return profile.Bundle{}, profileVerifyResponse{}, fmt.Errorf("profile audit failed for profile %q: %s", bundle.ID, profileaudit.FailureSummary(auditReport))
	}
	bundle, publish, err := importProfileBundle(ctx, runtime, profileImportRequest{
		Path:           path,
		Audit:          true,
		RequireAuditOK: true,
	})
	if err != nil {
		return profile.Bundle{}, profileVerifyResponse{}, err
	}
	verifyOptions := profileVerifyOptionsWithReadModels(options)
	checks, err := profileverify.PublishedChecks(ctx, runtime, bundle, profileverify.PublishedProfile{
		ProfileID:       publish.ProfileID,
		BundleDigest:    publish.BundleDigest,
		ConfigVersionID: publish.ConfigVersion.ID,
	}, verifyOptions)
	if err != nil {
		return profile.Bundle{}, profileVerifyResponse{}, err
	}
	report := profileVerifyResponse{
		OK:                profileverify.ChecksOK(checks),
		TemplatePackageID: bundle.ID,
		ProfileID:         bundle.ID,
		Audit:             *publish.Audit,
		Publish:           publish,
		Summary:           profileverify.Summarize(checks, verifyOptions),
		Checks:            checks,
	}
	if !report.OK {
		report.Error = fmt.Sprintf("profile verification failed for profile %q: %s", bundle.ID, profileverify.FirstFailed(checks))
		return profile.Bundle{}, report, errors.New(report.Error)
	}
	return bundle, report, nil
}

func profileVerifyOptionsWithReadModels(options profileVerifyOptions) profileVerifyOptions {
	if len(options.ReadModelKeys) == 0 {
		options.ReadModelKeys = []string{profilecatalog.ReadModelInterfaceNodes, ReadModelCatalog, ReadModelDashboard}
	}
	return options
}

func isProfileRequestError(err error) bool {
	message := err.Error()
	return strings.HasPrefix(message, "load profile") ||
		strings.HasPrefix(message, "digest profile") ||
		strings.HasPrefix(message, "audit profile") ||
		strings.HasPrefix(message, "profile audit failed")
}

func profileImportCountsFrom(counts profile.Counts) profileImportCounts {
	return profileImportCounts{
		Services:         counts.Services,
		Workflows:        counts.Workflows,
		InterfaceNodes:   counts.InterfaceNodes,
		APICases:         counts.APICases,
		RequestTemplates: counts.RequestTemplates,
		CaseDependencies: counts.CaseDependencies,
		WorkflowBindings: counts.WorkflowBindings,
		Fixtures:         counts.Fixtures,
	}
}

func profileImportStoreFromIndex(index store.ProfileIndex) profileImportStore {
	return profileImportStore{
		TemplatePackageID:     index.ProfileID,
		TemplatePackagePath:   index.BundlePath,
		TemplatePackageDigest: index.BundleDigest,
		ProfileID:             index.ProfileID,
		BundlePath:            index.BundlePath,
		BundleDigest:          index.BundleDigest,
		ImportedAt:            index.ImportedAt,
		UpdatedAt:             index.UpdatedAt,
	}
}

func profileImportConfigVersionFromStore(item store.ConfigVersion) profileImportConfigVersion {
	return profileImportConfigVersion{
		ID:                    item.ID,
		TemplatePackageID:     item.ProfileID,
		TemplatePackagePath:   item.SourcePath,
		TemplatePackageDigest: item.BundleDigest,
		ProfileID:             item.ProfileID,
		SourcePath:            item.SourcePath,
		BundleDigest:          item.BundleDigest,
		Active:                item.Active,
		PublishedAt:           item.PublishedAt,
		CreatedAt:             item.CreatedAt,
	}
}
