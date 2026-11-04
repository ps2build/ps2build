package ninja

import (
	"fmt"
	"io"
	"strings"
)

type Writer struct {
	w   io.Writer
	err error
}

// NewWriter returns a Writer that writes to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (w *Writer) Err() error {
	return w.err
}

func (w *Writer) printf(format string, args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.w, format, args...)
}

// Comment writes a '# text' comment line.
func (w *Writer) Comment(text string) {
	w.printf("# %s\n", text)
}

// Blank writes a blank line, used to visually separate sections.
func (w *Writer) Blank() {
	w.printf("\n")
}

// Variable writes a top-level 'name = value' assignment. value is escaped.
func (w *Writer) Variable(name, value string) {
	w.printf("%s = %s\n", name, escape(value))
}

func (w *Writer) Rule(name, command, description string) {
	w.printf("rule %s\n", name)
	w.printf("  command = %s\n", command)
	if description != "" {
		w.printf("  description = %s\n", description)
	}
}

func (w *Writer) RuleCompile(name, command, description string) {
	w.printf("rule %s\n", name)
	w.printf("  command = %s\n", command)
	w.printf("  depfile = $out.d\n")
	w.printf("  deps = gcc\n")
	if description != "" {
		w.printf("  description = %s\n", description)
	}
}

func (w *Writer) RuleCompileWithRspfile(name, command, rspfile, rspfileContent, description string) {
	w.printf("rule %s\n", name)
	w.printf("  command = %s\n", command)
	w.printf("  depfile = $out.d\n")
	w.printf("  deps = gcc\n")
	w.printf("  rspfile = %s\n", rspfile)
	w.printf("  rspfile_content = %s\n", rspfileContent)
	if description != "" {
		w.printf("  description = %s\n", description)
	}
}

func (w *Writer) RuleWithRspfile(name, command, rspfile, rspfileContent, description string) {
	w.printf("rule %s\n", name)
	w.printf("  command = %s\n", command)
	w.printf("  rspfile = %s\n", rspfile)
	w.printf("  rspfile_content = %s\n", rspfileContent)
	if description != "" {
		w.printf("  description = %s\n", description)
	}
}

// BuildEdge describes one 'build outputs: rule inputs' statement.
type BuildEdge struct {
	Outputs  []string
	Rule     string
	Inputs   []string
	Implicit []string // inputs listed after '|', tracked as dependencies but not passed as $in
	// Vars are edge-scoped variable assignments (e.g. "flags"), written in
	// the given order directly beneath the build line. Values are escaped.
	Vars [][2]string
}

// Build writes one BuildEdge.
func (w *Writer) Build(e BuildEdge) {
	outputs := joinEscaped(e.Outputs)
	line := fmt.Sprintf("build %s: %s", outputs, e.Rule)
	if len(e.Inputs) > 0 {
		line += " " + joinEscaped(e.Inputs)
	}
	if len(e.Implicit) > 0 {
		line += " | " + joinEscaped(e.Implicit)
	}
	w.printf("%s\n", line)
	for _, kv := range e.Vars {
		w.printf("  %s = %s\n", kv[0], escape(kv[1]))
	}
}

// Phony writes a 'build output: phony inputs...' alias edge.
func (w *Writer) Phony(output string, inputs []string) {
	w.Build(BuildEdge{Outputs: []string{output}, Rule: "phony", Inputs: inputs})
}

// Default writes a 'default targets...' statement.
func (w *Writer) Default(targets []string) {
	w.printf("default %s\n", joinEscaped(targets))
}

func escape(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	s = strings.ReplaceAll(s, " ", "$ ")
	return s
}

func escapePath(s string) string {
	s = escape(s)
	s = strings.ReplaceAll(s, ":", "$:")
	return s
}

func joinEscaped(paths []string) string {
	escaped := make([]string, len(paths))
	for i, p := range paths {
		escaped[i] = escapePath(p)
	}
	return strings.Join(escaped, " ")
}
