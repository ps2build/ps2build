package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func callValidate(t *testing.T, path string) map[string]any {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	registerValidateTarget(server)
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "validate_target", Arguments: map[string]any{"path": path}})
	if err != nil {
		t.Fatal(err)
	}
	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %#v", res.StructuredContent)
	}
	return out
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ps2.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateTargetListsUnknownKeyWarnings(t *testing.T) {
	path := writeYAML(t, "project: t\nfoo: 1\ntargets:\n  - name: m\n    type: ee\n    sources: [m.c]\n    bar: 2\n")
	out := callValidate(t, path)
	if out["valid"] != true {
		t.Fatalf("result = %v, want valid: unknown keys are only warnings", out)
	}
	got, _ := out["warnings"].([]any)
	want := []string{path + `: unknown key "foo" ignored`, path + `: unknown key "targets[0].bar" ignored`}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("warnings = %v, want %v", got, want)
	}
}

func TestValidateTargetWithoutUnknownKeysHasNoWarnings(t *testing.T) {
	out := callValidate(t, writeYAML(t, "project: t\ntargets:\n  - name: m\n    type: ee\n    sources: [m.c]\n"))
	if out["valid"] != true || out["warnings"] != nil {
		t.Errorf("result = %v, want valid with no warnings", out)
	}
}

func TestValidateTargetReportsANewerSchema(t *testing.T) {
	out := callValidate(t, writeYAML(t, "schema_version: 2\nproject: t\ntargets:\n  - name: m\n    type: ee\n    sources: [m.c]\n"))
	if out["valid"] != false || out["error"] == nil {
		t.Errorf("result = %v, want invalid with an error naming the schema version", out)
	}
}
