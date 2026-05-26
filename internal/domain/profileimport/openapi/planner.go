package openapi

import (
	"encoding/json"
	"strings"

	"agent-testbench/internal/domain/apicasespec"
	"agent-testbench/internal/domain/openapispec"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profiledraft"
)

type Options struct {
	ServiceID   string
	EvidenceDir string
}

type PlanResult struct {
	Service          profile.Service           `json:"service"`
	InterfaceNodes   []profile.InterfaceNode   `json:"interfaceNodes"`
	RequestTemplates []profile.RequestTemplate `json:"requestTemplates"`
	APICases         []profile.APICase         `json:"apiCases"`
	CaseFiles        []GeneratedCaseFile       `json:"caseFiles"`
}

type GeneratedCaseFile struct {
	Path string          `json:"path"`
	Body json.RawMessage `json:"body"`
}

func Plan(raw []byte, options Options) (PlanResult, error) {
	doc, err := openapispec.Decode(raw)
	if err != nil {
		return PlanResult{}, err
	}
	title := doc.Title()
	serviceID := openapispec.ServiceID(options.ServiceID, title)
	result := PlanResult{
		Service: profile.Service{
			ID:          serviceID,
			DisplayName: title,
			Kind:        "http",
			Status:      "draft",
		},
	}
	for _, opRef := range doc.Operations() {
		op := opRef.Operation
		opSlug := opRef.Slug()
		nodeID := "node." + serviceID + "." + opSlug
		caseID := "case." + serviceID + "." + opSlug
		templateID := "template." + serviceID + "." + opSlug
		displayName := opRef.DisplayName()
		statusCode := openapispec.FirstSuccessStatus(op.Responses)
		if statusCode == 0 {
			statusCode = 200
		}
		body := jsonExampleBody(op)

		result.InterfaceNodes = append(result.InterfaceNodes, profile.InterfaceNode{
			ID:          nodeID,
			DisplayName: displayName,
			ServiceID:   serviceID,
			Operation:   opRef.OperationName(),
			Method:      strings.ToUpper(opRef.Method),
			Path:        opRef.Path,
			Status:      "draft",
			Tags:        profiledraft.CompactTags(op.Tags),
			Description: op.Description,
			SortOrder:   len(result.InterfaceNodes) + 1,
		})
		result.RequestTemplates = append(result.RequestTemplates, profile.RequestTemplate{
			ID:           templateID,
			DisplayName:  displayName,
			NodeID:       nodeID,
			Method:       strings.ToUpper(opRef.Method),
			Path:         opRef.Path,
			TemplateJSON: profiledraft.CompactJSON(map[string]any{"method": strings.ToUpper(opRef.Method), "path": opRef.Path, "body": body}),
		})
		casePath := "api-cases/" + caseID + ".json"
		result.APICases = append(result.APICases, profile.APICase{
			ID:                caseID,
			DisplayName:       displayName,
			Description:       "Draft case generated from OpenAPI import plan.",
			NodeID:            nodeID,
			RequestTemplateID: templateID,
			Tags:              profiledraft.CompactTags(append([]string{"openapi"}, op.Tags...)),
			Status:            "draft",
			CasePath:          casePath,
			EvidenceDir:       strings.TrimSpace(options.EvidenceDir),
			SortOrder:         len(result.APICases) + 1,
		})
		result.CaseFiles = append(result.CaseFiles, GeneratedCaseFile{
			Path: casePath,
			Body: runnableCaseBody(caseID, displayName, opRef.Method, opRef.Path, body, statusCode),
		})
	}
	return result, nil
}

func jsonExampleBody(op openapispec.Operation) map[string]any {
	if op.RequestBody.Content == nil {
		return nil
	}
	for _, contentType := range []string{"application/json"} {
		if media, ok := op.RequestBody.Content[contentType]; ok {
			if body, ok := media.Example.(map[string]any); ok {
				return body
			}
		}
	}
	return nil
}

func runnableCaseBody(caseID string, title string, method string, path string, body map[string]any, statusCode int) json.RawMessage {
	var headers map[string]string
	if len(body) > 0 {
		headers = map[string]string{"Content-Type": "application/json"}
	}
	item := apicasespec.NewHTTPCase(caseID, title, strings.ToUpper(method), path, headers, body, statusCode)
	return apicasespec.JSON(item)
}
