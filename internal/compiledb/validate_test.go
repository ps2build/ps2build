package compiledb

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	good := Entry{Directory: "/o", File: "/src/a.c", Arguments: []string{"/tc/clang", "--target=mips64el-unknown-elf", "-isystem", "/inc", "-c", "/src/a.c"}}
	if p := Validate([]Entry{good}, ValidateOptions{}); len(p) != 0 {
		t.Fatalf("unexpected problems: %v", p)
	}
	bad := Entry{Directory: "o", File: "a.c", Arguments: []string{"clang", "-mno-gpopt", "-isystem", "/nonexistent-dir"}}
	p := strings.Join(Validate([]Entry{bad}, ValidateOptions{CheckDirs: true}), "\n")
	for _, want := range []string{"directory", "file", "compiler", "GCC-only", "no --target", "does not exist"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in:\n%s", want, p)
		}
	}
}
