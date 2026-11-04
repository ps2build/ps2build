package schema

import (
	"encoding/json"
	"testing"
)

func TestGenerate(t *testing.T) {
	s := Generate()

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshaling schema: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	// Spot-check that the yaml-tag-based field names came through (not the
	// Go struct field names) and that required-ness follows omitempty.
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("expected $defs in schema, got keys: %v", keys(doc))
	}
	target, ok := defs["Target"].(map[string]any)
	if !ok {
		t.Fatalf("expected $defs.Target in schema, got keys: %v", keys(defs))
	}
	props, ok := target["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected Target.properties, got: %v", target)
	}
	for _, want := range []string{"name", "type", "sources", "embed_irx"} {
		if _, ok := props[want]; !ok {
			t.Errorf("Target.properties missing %q, got: %v", want, keys(props))
		}
	}

	required, _ := target["required"].([]any)
	requiredSet := map[string]bool{}
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	for _, want := range []string{"name", "type", "sources"} {
		if !requiredSet[want] {
			t.Errorf("Target.required missing %q, got: %v", want, required)
		}
	}
	if requiredSet["embed_irx"] {
		t.Errorf("Target.required should not include optional field embed_irx, got: %v", required)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
