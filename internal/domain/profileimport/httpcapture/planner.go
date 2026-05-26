package httpcapture

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"agent-testbench/internal/domain/apicasespec"
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

type document struct {
	Name     string    `json:"name"`
	Captures []capture `json:"captures"`
}

type capture struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags"`
	Request  request  `json:"request"`
	Response response `json:"response"`
}

type request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

type response struct {
	Status int `json:"status"`
	Body   any `json:"body"`
}

func Plan(raw []byte, options Options) (PlanResult, error) {
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return PlanResult{}, fmt.Errorf("decode http capture document: %w", err)
	}
	if len(doc.Captures) == 0 {
		return PlanResult{}, errors.New("http capture document requires captures")
	}
	title := strings.TrimSpace(doc.Name)
	if title == "" {
		title = "HTTP Capture"
	}
	serviceID := strings.TrimSpace(options.ServiceID)
	if serviceID == "" {
		serviceID = "service." + profiledraft.Slug(title)
	}
	result := PlanResult{
		Service: profile.Service{
			ID:          serviceID,
			DisplayName: title,
			Kind:        "http",
			Status:      "draft",
		},
	}
	for index, item := range doc.Captures {
		method := strings.ToUpper(strings.TrimSpace(item.Request.Method))
		if method == "" {
			method = "GET"
		}
		path := strings.TrimSpace(item.Request.Path)
		if path == "" {
			path = "/"
		}
		opSlug := captureSlug(index, method, path, item)
		nodeID := "node." + serviceID + "." + opSlug
		caseID := "case." + serviceID + "." + opSlug
		templateID := "template." + serviceID + "." + opSlug
		displayName := profiledraft.FirstNonEmpty(item.Name, item.ID, method+" "+path)
		statusCode := item.Response.Status
		if statusCode == 0 {
			statusCode = 200
		}
		tags := profiledraft.CompactTags(append([]string{"recorded", "replay"}, item.Tags...))

		result.InterfaceNodes = append(result.InterfaceNodes, profile.InterfaceNode{
			ID:          nodeID,
			DisplayName: displayName,
			ServiceID:   serviceID,
			Operation:   profiledraft.FirstNonEmpty(item.ID, displayName),
			Method:      method,
			Path:        path,
			Status:      "draft",
			Tags:        tags,
			Description: "Draft interface generated from recorded HTTP capture.",
			SortOrder:   len(result.InterfaceNodes) + 1,
		})
		result.RequestTemplates = append(result.RequestTemplates, profile.RequestTemplate{
			ID:           templateID,
			DisplayName:  displayName,
			NodeID:       nodeID,
			Method:       method,
			Path:         path,
			TemplateJSON: profiledraft.CompactJSON(map[string]any{"method": method, "path": path, "body": item.Request.Body}),
		})
		casePath := "api-cases/" + caseID + ".json"
		result.APICases = append(result.APICases, profile.APICase{
			ID:                caseID,
			DisplayName:       displayName,
			Description:       "Draft replay case generated from recorded HTTP capture.",
			NodeID:            nodeID,
			RequestTemplateID: templateID,
			Tags:              tags,
			Status:            "draft",
			CasePath:          casePath,
			EvidenceDir:       strings.TrimSpace(options.EvidenceDir),
			SortOrder:         len(result.APICases) + 1,
		})
		result.CaseFiles = append(result.CaseFiles, GeneratedCaseFile{
			Path: casePath,
			Body: runnableCaseBody(caseID, displayName, method, path, item.Request.Headers, item.Request.Body, statusCode, item.Response.Body),
		})
	}
	return result, nil
}

func captureSlug(index int, method string, path string, item capture) string {
	if strings.TrimSpace(item.ID) != "" {
		return profiledraft.Slug(item.ID)
	}
	if strings.TrimSpace(item.Name) != "" {
		return profiledraft.Slug(item.Name)
	}
	return profiledraft.Slug(strings.ToLower(method) + "-" + path + "-" + strconv.Itoa(index+1))
}

func runnableCaseBody(caseID string, title string, method string, path string, headers map[string]string, body any, statusCode int, responseBody any) json.RawMessage {
	item := apicasespec.NewHTTPCase(caseID, title, method, path, headers, bodyMap(body), statusCode)
	if responseText := profiledraft.CompactJSON(responseBody); responseText != "{}" && responseText != "null" {
		item.Assertions.ResponseContains = []string{responseText}
	}
	return apicasespec.JSON(item)
}

func bodyMap(value any) map[string]any {
	if body, ok := value.(map[string]any); ok {
		return body
	}
	return nil
}
