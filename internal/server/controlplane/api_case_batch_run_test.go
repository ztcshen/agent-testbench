package controlplane

import "testing"

func TestNormalizeAPICaseBatchOverrideKey(t *testing.T) {
	tests := map[string]string{
		"itemId":       "item_id",
		"item_id":      "item_id",
		"ItemID":       "item_id",
		"HTTPStatus":   "http_status",
		"item-id":      "item_id",
		" item id ":    "item_id",
		"order.total1": "",
	}
	for input, want := range tests {
		if got := normalizeAPICaseBatchOverrideKey(input); got != want {
			t.Fatalf("normalizeAPICaseBatchOverrideKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAPICaseBatchCaseReportsExposeExecutionTimeoutBudget(t *testing.T) {
	plans := []apiCaseBatchCasePlan{
		{ID: "case.default", TimeoutSeconds: defaultAPICaseBatchTimeoutSeconds},
		{ID: "case.configured", TimeoutSeconds: 7},
	}
	reports := apiCaseBatchCaseReportsFromPlans(plans)
	if len(reports) != 2 || reports[0].TimeoutSeconds != 90 || reports[1].TimeoutSeconds != 7 {
		t.Fatalf("case timeout reports = %#v", reports)
	}
	if terminal := newAPICaseBatchCaseReport(plans[0]); terminal.TimeoutSeconds != 90 {
		t.Fatalf("terminal case timeout report = %#v", terminal)
	}
}
