// Package stepblock models versioned reusable workflow steps and their inputs.
package stepblock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Binding struct {
	Parameter    string `json:"parameter"`
	SourceStepID string `json:"sourceStepId"`
	From         string `json:"from"`
	Path         string `json:"path"`
}

type Output struct {
	Name string `json:"name"`
	From string `json:"from"`
	Path string `json:"path"`
}

type Step struct {
	ID             string               `json:"id"`
	Title          string               `json:"title"`
	CaseID         string               `json:"caseId"`
	Required       bool                 `json:"required"`
	Position       Position             `json:"position"`
	Bindings       []Binding            `json:"bindings"`
	Outputs        []Output             `json:"outputs"`
	Values         map[string]any       `json:"values,omitempty"`
	Variables      []string             `json:"variables,omitempty"`
	Request        map[string]any       `json:"request,omitempty"`
	Group          *Instance            `json:"group,omitempty"`
	ValueContracts map[string]Parameter `json:"valueContracts,omitempty"`
}

type Target struct {
	StepID    string `json:"stepId"`
	Parameter string `json:"parameter"`
}

type Parameter struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	DataType     string   `json:"dataType"`
	Required     bool     `json:"required"`
	Nullable     bool     `json:"nullable,omitempty"`
	HasDefault   bool     `json:"hasDefault"`
	DefaultValue any      `json:"defaultValue"`
	Targets      []Target `json:"targets"`
}

type Definition struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"displayName"`
	Description string      `json:"description"`
	Revision    string      `json:"revision"`
	Steps       []Step      `json:"steps"`
	Parameters  []Parameter `json:"parameters"`
}

type Instance struct {
	ID         string         `json:"id"`
	BlockID    string         `json:"blockId"`
	Revision   string         `json:"revision"`
	Title      string         `json:"title"`
	Values     map[string]any `json:"values"`
	Bindings   []Binding      `json:"bindings"`
	Parameters []Parameter    `json:"parameters,omitempty"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,159}$`)

func ValidID(value string) bool { return identifier.MatchString(value) }

func Canonical(def Definition) (Definition, error) {
	if !ValidID(def.ID) || strings.TrimSpace(def.DisplayName) == "" {
		return def, fmt.Errorf("step block requires a valid id and display name")
	}
	if len(def.Steps) == 0 || len(def.Steps) > 100 {
		return def, fmt.Errorf("step block must contain between 1 and 100 steps")
	}
	steps := map[string]bool{}
	for index := range def.Steps {
		s := &def.Steps[index]
		if !ValidID(s.ID) || steps[s.ID] || s.CaseID == "" || s.Group != nil {
			return def, fmt.Errorf("step block requires unique steps with cases; nested groups are not supported")
		}
		for _, binding := range s.Bindings {
			if !steps[binding.SourceStepID] {
				return def, fmt.Errorf("step %s has a binding outside its preceding block steps", s.ID)
			}
		}
		steps[s.ID] = true
		s.Variables, s.Request, s.ValueContracts = nil, nil, nil
	}
	names, targets := map[string]bool{}, map[string]bool{}
	for index := range def.Parameters {
		p := &def.Parameters[index]
		if !ValidID(p.Name) || names[p.Name] || len(p.Targets) == 0 {
			return def, fmt.Errorf("step block parameters require unique names and at least one target")
		}
		names[p.Name] = true
		if p.DataType == "" {
			p.DataType = "string"
		}
		if !ValidType(p.DataType) {
			return def, fmt.Errorf("parameter %s has an unsupported type", p.Name)
		}
		if p.HasDefault {
			if err := ValidateValue(*p, p.DefaultValue); err != nil {
				return def, err
			}
		} else {
			p.DefaultValue = nil
		}
		for _, target := range p.Targets {
			key := target.StepID + ":" + target.Parameter
			if !steps[target.StepID] || target.Parameter == "" || targets[key] {
				return def, fmt.Errorf("parameter %s has an invalid or repeated step target", p.Name)
			}
			targets[key] = true
		}
	}
	def.Revision = ""
	raw, err := json.Marshal(def)
	if err != nil {
		return def, fmt.Errorf("encode step block: %w", err)
	}
	digest := sha256.Sum256(raw)
	def.Revision = hex.EncodeToString(digest[:])
	return def, nil
}

func ValidType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	}
	return false
}

func ValidateValue(p Parameter, value any) error {
	valid := value == nil && p.Nullable
	if value != nil {
		switch p.DataType {
		case "string":
			_, valid = value.(string)
		case "boolean":
			_, valid = value.(bool)
		case "number", "integer":
			v, ok := value.(float64)
			valid = ok && !math.IsNaN(v) && !math.IsInf(v, 0) && (p.DataType != "integer" || math.Trunc(v) == v)
		case "object":
			_, valid = value.(map[string]any)
		case "array":
			_, valid = value.([]any)
		}
	}
	if !valid {
		return fmt.Errorf("parameter %s must be %s%s", p.Name, p.DataType, nullableLabel(p.Nullable))
	}
	return nil
}

func nullableLabel(nullable bool) string {
	if nullable {
		return " or null"
	}
	return ""
}

// CoerceBinding accepts the text representation of existing response exports.
// Literal arguments still use ValidateValue without implicit conversion.
func CoerceBinding(p Parameter, value any) (any, error) {
	if text, ok := value.(string); ok && p.DataType != "string" {
		switch p.DataType {
		case "number", "integer":
			if number, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
				value = number
			}
		case "boolean":
			if text == "true" {
				value = true
			} else if text == "false" {
				value = false
			}
		case "object", "array":
			var decoded any
			if json.Unmarshal([]byte(text), &decoded) == nil {
				value = decoded
			}
		}
	}
	return value, ValidateValue(p, value)
}
