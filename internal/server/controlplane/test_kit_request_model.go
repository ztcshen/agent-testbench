package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"agent-testbench/internal/domain/profile"
)

func joinCaseURL(baseURL string, path string, query map[string]any) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	pathURL, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	parsed = parsed.ResolveReference(pathURL)
	values := parsed.Query()
	for key, raw := range query {
		if value := strings.TrimSpace(valueString(raw)); value != "" {
			values.Set(key, value)
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func renderCaseExecution(execution caseExecutionConfig, overrides map[string]any) caseExecutionConfig {
	rendered := execution
	rendered.Path = valueString(renderCaseExecutionValue(rendered.Path, overrides))
	rendered.Query = mapFromAny(renderCaseExecutionValue(rendered.Query, overrides))
	rendered.Headers = mapFromAny(renderCaseExecutionValue(rendered.Headers, overrides))
	rendered.Auth = mapFromAny(renderCaseExecutionValue(rendered.Auth, overrides))
	rendered.TraceEndpoint = valueString(renderCaseExecutionValue(rendered.TraceEndpoint, overrides))
	rendered.Body = renderCaseExecutionValue(rendered.Body, overrides)
	return rendered
}

func applyAPICaseRequestModel(request *caseHTTPRequest, item profile.APICase) error {
	if request == nil {
		return nil
	}
	if err := applyAPICaseExpectedJSON(request, item.ExpectedJSON); err != nil {
		return err
	}
	if strings.TrimSpace(item.RenderMode) != "template_patch" || strings.TrimSpace(item.PatchJSON) == "" || strings.TrimSpace(item.PatchJSON) == "[]" {
		return nil
	}
	if apiCasePatchTargetsQuery(request.method) {
		return applyAPICaseQueryPatch(request, item)
	}
	nextBody := request.body
	if apiCaseUsesSandboxCallback(request.fullURL) {
		merged, err := mergeAPICasePayloadTemplateModel(nextBody, item.PayloadTemplateJSON)
		if err != nil {
			return fmt.Errorf("merge api case payload template %s: %w", item.ID, err)
		}
		nextBody = merged
	}
	if nextBody == nil && strings.TrimSpace(item.PayloadTemplateJSON) != "" {
		var parsed any
		if err := json.Unmarshal([]byte(item.PayloadTemplateJSON), &parsed); err != nil {
			return fmt.Errorf("decode api case payload template %s: %w", item.ID, err)
		}
		nextBody = parsed
	}
	if nextBody == nil {
		return nil
	}
	patched, err := applyAPICaseJSONPatch(nextBody, item.PatchJSON)
	if err != nil {
		return fmt.Errorf("apply api case patch %s: %w", item.ID, err)
	}
	if err := applyAPICaseEquivalentBodyPatch(patched, item.PatchJSON); err != nil {
		return fmt.Errorf("apply api case equivalent field patch %s: %w", item.ID, err)
	}
	request.body = patched
	if err := resignAPICaseRequest(request, item.ID); err != nil {
		return err
	}
	return nil
}

func apiCaseUsesSandboxCallback(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, "/__sandbox/llt/callback")
	}
	return parsed.Path == "/__sandbox/llt/callback"
}

func mergeAPICasePayloadTemplateModel(body any, templateJSON string) (any, error) {
	templateJSON = strings.TrimSpace(templateJSON)
	if templateJSON == "" || templateJSON == "{}" {
		return body, nil
	}
	var templateModel any
	if err := json.Unmarshal([]byte(templateJSON), &templateModel); err != nil {
		return nil, err
	}
	templateObject := mapFromAny(renderCaseExecutionValue(templateModel, nil))
	if len(templateObject) == 0 {
		return body, nil
	}
	bodyObject, ok := body.(map[string]any)
	if !ok {
		return templateObject, nil
	}
	merged := make(map[string]any, len(templateObject)+len(bodyObject))
	for key, value := range templateObject {
		merged[key] = value
	}
	for key, value := range bodyObject {
		merged[key] = value
	}
	return merged, nil
}

func apiCasePatchTargetsQuery(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead:
		return true
	default:
		return false
	}
}

func applyAPICaseQueryPatch(request *caseHTTPRequest, item profile.APICase) error {
	parsed, queryObject, err := apiCaseQueryObject(request.fullURL)
	if err != nil {
		return fmt.Errorf("decode api case query %s: %w", item.ID, err)
	}
	if len(queryObject) == 0 && strings.TrimSpace(item.PayloadTemplateJSON) != "" {
		var template any
		if err := json.Unmarshal([]byte(item.PayloadTemplateJSON), &template); err != nil {
			return fmt.Errorf("decode api case payload template %s: %w", item.ID, err)
		}
		queryObject = mapFromAny(template)
	}
	patched, err := applyAPICaseJSONPatch(queryObject, item.PatchJSON)
	if err != nil {
		return fmt.Errorf("apply api case patch %s: %w", item.ID, err)
	}
	patchedQuery, ok := patched.(map[string]any)
	if !ok {
		return fmt.Errorf("api case patch %s must keep query as an object", item.ID)
	}
	parsed.RawQuery = apiCaseQueryValues(patchedQuery).Encode()
	request.fullURL = parsed.String()
	request.body = nil
	if err := resignAPICaseRequest(request, item.ID); err != nil {
		return err
	}
	return nil
}

func apiCaseQueryObject(rawURL string) (*url.URL, map[string]any, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	values := parsed.Query()
	out := make(map[string]any, len(values))
	for key, items := range values {
		if len(items) == 1 {
			out[key] = items[0]
			continue
		}
		array := make([]any, 0, len(items))
		for _, item := range items {
			array = append(array, item)
		}
		out[key] = array
	}
	return parsed, out, nil
}

func apiCaseQueryValues(query map[string]any) url.Values {
	values := url.Values{}
	for key, raw := range query {
		switch typed := raw.(type) {
		case nil:
			continue
		case []any:
			for _, item := range typed {
				values.Add(key, valueString(item))
			}
		case []string:
			for _, item := range typed {
				values.Add(key, item)
			}
		default:
			values.Set(key, valueString(typed))
		}
	}
	return values
}

func resignAPICaseRequest(request *caseHTTPRequest, caseID string) error {
	if !request.signed {
		return nil
	}
	if request.headers == nil {
		request.headers = map[string]string{}
	}
	delete(request.headers, "Authorization")
	if err := request.applySigning(); err != nil {
		return fmt.Errorf("sign patched api case request %s: %w", caseID, err)
	}
	return nil
}
