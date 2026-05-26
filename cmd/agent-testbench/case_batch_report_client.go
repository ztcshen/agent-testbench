package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"agent-testbench/internal/domain/profile"
)

func postTestKitRunBatch(serverURL string, cases []profile.APICase, baseURL string, timeoutSeconds int, failureLabel string) (map[string]any, error) {
	caseIDs := make([]string, 0, len(cases))
	for _, item := range cases {
		caseIDs = append(caseIDs, item.ID)
	}
	requestPayload := map[string]any{"caseIds": caseIDs, "baseUrl": baseURL, "timeoutSeconds": timeoutSeconds}
	rawRequest, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, err
	}
	response, err := http.Post(serverURL+"/api/test-kit/run-batch", "application/json", strings.NewReader(string(rawRequest)))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close test-kit batch response body: %v\n", closeErr)
		}
	}()
	var rawBatch map[string]any
	if err := json.NewDecoder(response.Body).Decode(&rawBatch); err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s failed with http status %d", failureLabel, response.StatusCode)
	}
	return rawBatch, nil
}
