package explain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ps2build/ps2build/internal/schema"
)

// Example is one embedded, golden-tested ps2.yaml sample.
type Example struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	YAML        string `json:"yaml"`
}

// Doc is the full explain document.
type Doc struct {
	Overview      string    `json:"overview"`
	TargetTypes   string    `json:"target_types"`
	CLICommands   string    `json:"cli_commands"`
	EnvVars       string    `json:"env_vars"`
	V1Scope       string    `json:"v1_scope"`
	PS2YAMLFormat any       `json:"ps2_yaml_format"`
	Examples      []Example `json:"examples"`
}

// sectionOrder is the canonical display/lookup order, also the set of
// valid --section values.
var sectionOrder = []string{
	"overview",
	"target_types",
	"cli_commands",
	"env_vars",
	"v1_scope",
	"ps2_yaml_format",
	"examples",
}

func BuildDoc() Doc {
	exs := make([]Example, len(examples))
	for i, e := range examples {
		exs[i] = Example{Name: e.Name, Description: e.Description, YAML: e.YAML}
	}

	return Doc{
		Overview:      strings.TrimSpace(overview),
		TargetTypes:   strings.TrimSpace(targetTypes),
		CLICommands:   strings.TrimSpace(cliCommands),
		EnvVars:       strings.TrimSpace(envVars),
		V1Scope:       strings.TrimSpace(v1Scope),
		PS2YAMLFormat: schema.Generate(),
		Examples:      exs,
	}
}

// ValidSections returns the accepted --section values, in display order.
func ValidSections() []string {
	out := make([]string, len(sectionOrder))
	copy(out, sectionOrder)
	return out
}

// RenderJSON marshals the doc (or, if section is non-empty, just that one
// field) as indented JSON.
func RenderJSON(d Doc, section string) ([]byte, error) {
	if section == "" {
		return json.MarshalIndent(d, "", "  ")
	}
	v, err := field(d, section)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(v, "", "  ")
}

// RenderText renders the doc (or just one section) as human-readable text.
func RenderText(d Doc, section string) (string, error) {
	if section != "" {
		v, err := field(d, section)
		if err != nil {
			return "", err
		}
		return renderSectionText(section, v), nil
	}

	var b strings.Builder
	for _, name := range sectionOrder {
		v, _ := field(d, name)
		b.WriteString(renderSectionText(name, v))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func renderSectionText(name string, v any) string {
	title := strings.ToUpper(strings.ReplaceAll(name, "_", " "))
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", title)

	switch name {
	case "ps2_yaml_format":
		data, _ := json.MarshalIndent(v, "", "  ")
		b.Write(data)
	case "examples":
		exs, _ := v.([]Example)
		for _, e := range exs {
			fmt.Fprintf(&b, "\n--- %s: %s ---\n%s", e.Name, e.Description, e.YAML)
		}
	default:
		fmt.Fprintf(&b, "%v", v)
	}
	return b.String()
}

func field(d Doc, section string) (any, error) {
	switch section {
	case "overview":
		return d.Overview, nil
	case "target_types", "targets":
		return d.TargetTypes, nil
	case "cli_commands":
		return d.CLICommands, nil
	case "env_vars":
		return d.EnvVars, nil
	case "v1_scope":
		return d.V1Scope, nil
	case "ps2_yaml_format":
		return d.PS2YAMLFormat, nil
	case "examples":
		return d.Examples, nil
	default:
		return nil, fmt.Errorf("unknown section %q, valid sections: %s", section, strings.Join(sortedCopy(sectionOrder), ", "))
	}
}

func sortedCopy(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}
