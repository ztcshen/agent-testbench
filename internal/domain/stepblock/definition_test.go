package stepblock

import "testing"

func TestDefinitionRevisionAndParameterContracts(t *testing.T) {
	def := Definition{ID: "block.create", DisplayName: "Create", Steps: []Step{{ID: "step.one", CaseID: "case.create"}}, Parameters: []Parameter{{Name: "count", DataType: "integer", Required: true, HasDefault: true, DefaultValue: float64(0), Targets: []Target{{StepID: "step.one", Parameter: "count"}}}}}
	first, err := Canonical(def)
	if err != nil {
		t.Fatal(err)
	}
	def.Description = "Updated"
	second, err := Canonical(def)
	if err != nil || first.Revision == second.Revision {
		t.Fatal("definition revision did not change")
	}
	if ValidateValue(def.Parameters[0], "0") == nil {
		t.Fatal("literal numeric strings must not satisfy an integer contract")
	}
	if value, err := CoerceBinding(def.Parameters[0], "0"); err != nil || value != float64(0) {
		t.Fatalf("exported numeric value was not coerced: %v %v", value, err)
	}
	if _, err := CoerceBinding(def.Parameters[0], "not-a-number"); err == nil {
		t.Fatal("invalid upstream number was accepted")
	}
	boolean := Parameter{Name: "enabled", DataType: "boolean"}
	if ValidateValue(boolean, false) != nil {
		t.Fatal("false must remain a valid supplied value")
	}
}

func TestDefinitionRejectsEscapingBindingsAndDuplicateTargets(t *testing.T) {
	def := Definition{ID: "block.invalid", DisplayName: "Invalid", Steps: []Step{{ID: "step.one", CaseID: "case.one", Bindings: []Binding{{Parameter: "id", SourceStepID: "outside"}}}}}
	if _, err := Canonical(def); err == nil {
		t.Fatal("block retained a dependency outside its definition")
	}
	def.Steps[0].Bindings = nil
	def.Parameters = []Parameter{{Name: "id", DataType: "string", Targets: []Target{{StepID: "step.one", Parameter: "id"}, {StepID: "step.one", Parameter: "id"}}}}
	if _, err := Canonical(def); err == nil {
		t.Fatal("duplicate target was accepted")
	}
}
