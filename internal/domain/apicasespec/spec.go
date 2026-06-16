// Package apicasespec defines neutral API case request specifications.
package apicasespec

import "encoding/json"

type Case struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Request    Request    `json:"request"`
	Assertions Assertions `json:"assertions"`
}

type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    map[string]any    `json:"body,omitempty"`
}

type Assertions struct {
	ExpectedStatusCodes []int    `json:"expectedStatusCodes,omitempty"`
	ResponseContains    []string `json:"responseContains,omitempty"`
	ResponseNotContains []string `json:"responseNotContains,omitempty"`
}

func NewHTTPCase(id string, title string, method string, path string, headers map[string]string, body map[string]any, statusCode int) Case {
	return Case{
		ID:    id,
		Title: title,
		Request: Request{
			Method:  method,
			Path:    path,
			Headers: headers,
			Body:    body,
		},
		Assertions: Assertions{
			ExpectedStatusCodes: []int{statusCode},
		},
	}
}

func JSON(item Case) json.RawMessage {
	raw, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(append(raw, '\n'))
}
