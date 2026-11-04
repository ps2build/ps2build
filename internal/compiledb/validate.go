package compiledb

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// gccOnlyFlags are rejected by clang.
var gccOnlyFlags = map[string]bool{
	"-mno-gpopt":          true,
	"-mgpopt":             true,
	"-fno-gcse":           true,
	"-fno-ipa-cp":         true,
	"-fno-tree-dce":       true,
	"-fno-tree-vectorize": true,
}

// ValidateOptions tunes Validate.
type ValidateOptions struct {
	// CheckDirs requires every -isystem directory to exist on disk.
	CheckDirs bool
}

// Read parses a compile_commands.json document.
func Read(r io.Reader) ([]Entry, error) {
	var entries []Entry
	if err := json.NewDecoder(r).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func isAbs(p string) bool {
	p = strings.ReplaceAll(p, "\\", "/")
	// "<PLACEHOLDER>/..." is what golden fixtures use for absolute roots.
	return path.IsAbs(p) || filepath.IsAbs(p) || strings.HasPrefix(p, "<") ||
		(len(p) > 2 && p[1] == ':' && p[2] == '/')
}

// Validate returns one message per problem found; empty means valid.
func Validate(entries []Entry, opts ValidateOptions) []string {
	var problems []string
	add := func(i int, e Entry, format string, a ...any) {
		problems = append(problems, fmt.Sprintf("entry %d (%s): %s", i, e.File, fmt.Sprintf(format, a...)))
	}
	for i, e := range entries {
		if len(e.Arguments) == 0 {
			add(i, e, "no arguments")
			continue
		}
		if !isAbs(e.Directory) {
			add(i, e, "directory %q is not absolute", e.Directory)
		}
		if !isAbs(e.File) {
			add(i, e, "file %q is not absolute", e.File)
		}
		compiler := e.Arguments[0]
		if !isAbs(compiler) {
			add(i, e, "compiler %q is not an absolute path", compiler)
		}
		base := strings.ToLower(path.Base(strings.ReplaceAll(compiler, "\\", "/")))
		clang := strings.Contains(base, "clang")
		hasTarget := false
		for j := 1; j < len(e.Arguments); j++ {
			a := e.Arguments[j]
			switch {
			case strings.HasPrefix(a, "--target=") || strings.HasPrefix(a, "-target"):
				hasTarget = true
			case clang && gccOnlyFlags[a]:
				add(i, e, "GCC-only flag %s under clang", a)
			case a == "-isystem" && j+1 < len(e.Arguments):
				j++
				checkDir(opts, add, i, e, e.Arguments[j])
			case strings.HasPrefix(a, "-isystem") && len(a) > len("-isystem"):
				checkDir(opts, add, i, e, a[len("-isystem"):])
			}
		}
		if clang && !hasTarget {
			add(i, e, "clang invocation has no --target")
		}
	}
	return problems
}

func checkDir(opts ValidateOptions, add func(int, Entry, string, ...any), i int, e Entry, dir string) {
	if !isAbs(dir) {
		add(i, e, "-isystem %q is not absolute", dir)
		return
	}
	if opts.CheckDirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			add(i, e, "-isystem dir %q does not exist", dir)
		}
	}
}
