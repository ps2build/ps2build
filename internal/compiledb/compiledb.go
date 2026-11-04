package compiledb

import (
	"encoding/json"
	"io"
	"path"
	"strings"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

// Entry is one JSON Compilation Database entry.
type Entry struct {
	Directory string   `json:"directory"`
	File      string   `json:"file"`
	Output    string   `json:"output,omitempty"`
	Arguments []string `json:"arguments"`
}

func Entries(g *buildgraph.Graph) []Entry {
	entries := make([]Entry, 0, len(g.Compiles))
	isystemCache := map[string][]string{}
	for _, c := range g.Compiles {
		compiler := c.Toolchain
		if resolved, ok := g.ToolchainPaths[c.Toolchain]; ok {
			compiler = resolved
		}

		var dirs []string
		if !c.Assembler && !c.VCL {
			cached, ok := isystemCache[compiler]
			if !ok {
				cached = discoverImplicitIncludeDirs(compiler)
				isystemCache[compiler] = cached
			}
			dirs = cached
		}

		args := make([]string, 0, len(c.FlagArgs)+4+2*len(dirs))
		args = append(args, compiler)
		args = append(args, c.FlagArgs...)
		for _, dir := range dirs {
			args = append(args, "-isystem", dir)
		}
		args = dedupeIncludeArgs(args)
		if c.Assembler {
			args = append(args, c.Source, "-o", path.Join(g.OutDir, c.Object))
		} else {
			args = append(args, "-c", c.Source, "-o", path.Join(g.OutDir, c.Object))
		}

		entries = append(entries, Entry{
			Directory: g.OutDir,
			File:      c.Source,
			Output:    c.Object,
			Arguments: args,
		})
	}
	return entries
}

func includeDir(arg string) (flag, dir string, ok bool) {
	for _, f := range []string{"-isystem", "-I"} {
		if strings.HasPrefix(arg, f) {
			return f, arg[len(f):], true
		}
	}
	return "", "", false
}

// dedupeIncludeArgs drops repeated include directories, comparing cleaned forward-slash paths.
func dedupeIncludeArgs(args []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		flag, dir, ok := includeDir(args[i])
		if !ok {
			out = append(out, args[i])
			continue
		}
		split := dir == ""
		if split {
			if i+1 >= len(args) {
				out = append(out, args[i])
				continue
			}
			i++
			dir = args[i]
		}
		dir = path.Clean(strings.ReplaceAll(dir, "\\", "/"))
		if seen[flag+dir] {
			continue
		}
		seen[flag+dir] = true
		if split {
			out = append(out, flag, dir)
		} else {
			out = append(out, flag+dir)
		}
	}
	return out
}

// Write marshals entries as a pretty-printed JSON array to w.
func Write(entries []Entry, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}
