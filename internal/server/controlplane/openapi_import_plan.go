package controlplane

import (
	"net/http"
	"os"
	"strings"

	profilegenerateopenapi "agent-testbench/internal/domain/profilegenerate/openapi"
	profileimporthttpcapture "agent-testbench/internal/domain/profileimport/httpcapture"
	profileimportopenapi "agent-testbench/internal/domain/profileimport/openapi"
)

type planSourceInput struct {
	SourcePath  string
	ServiceID   string
	EvidenceDir string
	Raw         []byte
}

func handleOpenAPIImportPlan(w http.ResponseWriter, r *http.Request) {
	input, ok := readPlanSourceInput(w, r)
	if !ok {
		return
	}
	plan, err := profileimportopenapi.Plan(input.Raw, profileimportopenapi.Options{
		ServiceID:   input.ServiceID,
		EvidenceDir: input.EvidenceDir,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writePlanSourceResponse(w, "openapi", input.SourcePath, plan)
}

func handleOpenAPIGenerationPlan(w http.ResponseWriter, r *http.Request) {
	input, ok := readPlanSourceInput(w, r)
	if !ok {
		return
	}
	plan, err := profilegenerateopenapi.Plan(input.Raw, profilegenerateopenapi.Options{
		ServiceID:   input.ServiceID,
		EvidenceDir: input.EvidenceDir,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writePlanSourceResponse(w, "openapi", input.SourcePath, plan)
}

func handleHTTPCaptureImportPlan(w http.ResponseWriter, r *http.Request) {
	input, ok := readPlanSourceInput(w, r)
	if !ok {
		return
	}
	plan, err := profileimporthttpcapture.Plan(input.Raw, profileimporthttpcapture.Options{
		ServiceID:   input.ServiceID,
		EvidenceDir: input.EvidenceDir,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writePlanSourceResponse(w, "http-capture", input.SourcePath, plan)
}

func readPlanSourceInput(w http.ResponseWriter, r *http.Request) (planSourceInput, bool) {
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return planSourceInput{}, false
	}
	sourcePath := strings.TrimSpace(valueString(payload["sourcePath"]))
	if sourcePath == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "sourcePath is required"})
		return planSourceInput{}, false
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return planSourceInput{}, false
	}
	return planSourceInput{
		SourcePath:  sourcePath,
		ServiceID:   valueString(payload["serviceId"]),
		EvidenceDir: valueString(payload["evidenceDir"]),
		Raw:         raw,
	}, true
}

func writePlanSourceResponse(w http.ResponseWriter, kind string, sourcePath string, plan any) {
	writeJSON(w, map[string]any{
		"ok":         true,
		"kind":       kind,
		"sourcePath": sourcePath,
		"plan":       plan,
	})
}
