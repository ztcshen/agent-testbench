package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"agent-testbench/internal/domain/casemaintenance"
	"agent-testbench/internal/store"
)

type caseCatalogCasePatch struct {
	ID                   string    `json:"id"`
	DisplayName          *string   `json:"displayName"`
	Description          *string   `json:"description"`
	NodeID               *string   `json:"nodeId"`
	CaseType             *string   `json:"caseType"`
	Scenario             *string   `json:"scenario"`
	Tags                 *[]string `json:"tags"`
	Priority             *string   `json:"priority"`
	Owner                *string   `json:"owner"`
	RequestTemplateID    *string   `json:"requestTemplateId"`
	RenderMode           *string   `json:"renderMode"`
	RequiredForAdmission *bool     `json:"requiredForAdmission"`
	Status               *string   `json:"status"`
	SortOrder            *int      `json:"sortOrder"`
	CasePath             *string   `json:"casePath"`
	SourceKind           *string   `json:"sourceKind"`
	SourcePath           *string   `json:"sourcePath"`
	ExecutorID           *string   `json:"executorId"`
	BaseURL              *string   `json:"baseUrl"`
	EvidenceDir          *string   `json:"evidenceDir"`
	TimeoutSeconds       *int      `json:"timeoutSeconds"`
}

var caseCatalogPatchFields = map[string]struct{}{
	"id": {}, "displayName": {}, apiFieldDescription: {}, "nodeId": {}, "caseType": {},
	"scenario": {}, "tags": {}, "priority": {}, "owner": {}, "requestTemplateId": {},
	"renderMode": {}, "requiredForAdmission": {}, "status": {}, "sortOrder": {},
	"casePath": {}, "sourceKind": {}, "sourcePath": {}, "executorId": {}, "baseUrl": {},
	apiFieldEvidenceDir: {}, apiFieldTimeoutSeconds: {},
}

func patchCaseCatalog(catalogValue store.ProfileCatalog, payload map[string]any) (store.ProfileCatalog, store.CatalogAPICase, bool, error) {
	rawPatch, err := caseCatalogPatchObject(payload)
	if err != nil {
		return store.ProfileCatalog{}, store.CatalogAPICase{}, false, err
	}
	for field := range rawPatch {
		if _, ok := caseCatalogPatchFields[field]; !ok {
			return store.ProfileCatalog{}, store.CatalogAPICase{}, false, fmt.Errorf("unsupported case field %q", field)
		}
	}
	raw, err := json.Marshal(rawPatch)
	if err != nil {
		return store.ProfileCatalog{}, store.CatalogAPICase{}, false, err
	}
	var patch caseCatalogCasePatch
	if err := json.Unmarshal(raw, &patch); err != nil {
		return store.ProfileCatalog{}, store.CatalogAPICase{}, false, fmt.Errorf("decode case patch: %w", err)
	}
	patch.ID = strings.TrimSpace(patch.ID)
	if patch.ID == "" {
		return store.ProfileCatalog{}, store.CatalogAPICase{}, false, errors.New("case id is required")
	}

	caseIndex := -1
	candidate := store.CatalogAPICase{ID: patch.ID}
	for index, existing := range catalogValue.APICases {
		if strings.TrimSpace(existing.ID) == patch.ID {
			caseIndex = index
			candidate = existing
			break
		}
	}
	created := caseIndex < 0
	applyCaseCatalogPatch(&candidate, patch)
	candidate.ID = patch.ID
	status, err := casemaintenance.NormalizeCaseStatus(candidate.Status, created)
	if err != nil {
		return store.ProfileCatalog{}, store.CatalogAPICase{}, false, err
	}
	candidate.Status = status

	if created {
		catalogValue.APICases = append(catalogValue.APICases, candidate)
	} else {
		catalogValue.APICases[caseIndex] = candidate
	}
	if err := casemaintenance.ValidateCase(catalogValue, candidate); err != nil {
		return store.ProfileCatalog{}, store.CatalogAPICase{}, false, err
	}
	return catalogValue, candidate, created, nil
}

func caseCatalogPatchObject(payload map[string]any) (map[string]any, error) {
	value, ok := payload["case"]
	if !ok {
		return nil, errors.New("case object is required")
	}
	if len(payload) != 1 {
		return nil, errors.New("case is the only supported top-level field")
	}
	patch, ok := value.(map[string]any)
	if !ok || patch == nil {
		return nil, errors.New("case must be an object")
	}
	return patch, nil
}

func applyCaseCatalogPatch(candidate *store.CatalogAPICase, patch caseCatalogCasePatch) {
	setCaseCatalogString(&candidate.DisplayName, patch.DisplayName, false)
	setCaseCatalogString(&candidate.Description, patch.Description, false)
	setCaseCatalogString(&candidate.NodeID, patch.NodeID, true)
	setCaseCatalogString(&candidate.CaseType, patch.CaseType, true)
	setCaseCatalogString(&candidate.Scenario, patch.Scenario, false)
	if patch.Tags != nil {
		candidate.Tags = append([]string(nil), (*patch.Tags)...)
	}
	setCaseCatalogString(&candidate.Priority, patch.Priority, true)
	setCaseCatalogString(&candidate.Owner, patch.Owner, true)
	setCaseCatalogString(&candidate.RequestTemplateID, patch.RequestTemplateID, true)
	setCaseCatalogString(&candidate.RenderMode, patch.RenderMode, true)
	if patch.RequiredForAdmission != nil {
		candidate.RequiredForAdmission = *patch.RequiredForAdmission
	}
	setCaseCatalogString(&candidate.Status, patch.Status, true)
	if patch.SortOrder != nil {
		candidate.SortOrder = *patch.SortOrder
	}
	setCaseCatalogString(&candidate.CasePath, patch.CasePath, true)
	setCaseCatalogString(&candidate.SourceKind, patch.SourceKind, true)
	setCaseCatalogString(&candidate.SourcePath, patch.SourcePath, true)
	setCaseCatalogString(&candidate.ExecutorID, patch.ExecutorID, true)
	setCaseCatalogString(&candidate.BaseURL, patch.BaseURL, true)
	setCaseCatalogString(&candidate.EvidenceDir, patch.EvidenceDir, true)
	if patch.TimeoutSeconds != nil {
		candidate.TimeoutSeconds = *patch.TimeoutSeconds
	}
}

func setCaseCatalogString(target *string, value *string, trim bool) {
	if value == nil {
		return
	}
	*target = *value
	if trim {
		*target = strings.TrimSpace(*target)
	}
}

func writeCaseCatalogPatchError(w http.ResponseWriter, err error) {
	var validationErr *casemaintenance.ValidationError
	if errors.As(err, &validationErr) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"ok":     false,
			"error":  "case maintenance validation failed",
			"issues": validationErr.Issues,
		})
		return
	}
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
}
