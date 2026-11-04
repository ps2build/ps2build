package buildrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ps2build/ps2build/internal/utils"
)

func TestFindNinja(t *testing.T) {
	name := utils.ExeSuffix("ninja")

	toolsDir := utils.SDKPath("tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toolsNinja := filepath.Join(toolsDir, name)
	if err := os.WriteFile(toolsNinja, []byte("tools, should win"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindNinja()
	if err != nil {
		t.Fatalf("FindNinja() error = %v, want nil", err)
	}
	if want := filepath.ToSlash(toolsNinja); got != want {
		t.Errorf("FindNinja() = %q, want %q (tools/, not the sibling)", got, want)
	}

	os.RemoveAll(toolsDir)
}

func TestFindNinja_NotFoundAnywhere(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty - nothing named ninja here

	_, err := FindNinja()
	if err == nil {
		t.Fatal("FindNinja() error = nil, want an error when ninja isn't found anywhere")
	}
}
