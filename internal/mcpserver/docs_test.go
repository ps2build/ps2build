package mcpserver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/explain"
)

func TestExplainSectionHelpNamesEverySectionAndNoMissingFlag(t *testing.T) {
	field, ok := reflect.TypeOf(explainArgs{}).FieldByName("Section")
	if !ok {
		t.Fatal("explainArgs has no Section field")
	}
	help := field.Tag.Get("jsonschema")
	for _, name := range explain.ValidSections() {
		if !strings.Contains(help, name) {
			t.Errorf("section help %q does not name %q", help, name)
		}
	}
	if strings.Contains(help, "--list") {
		t.Errorf("section help %q points at ps2build explain --list, which does not exist", help)
	}
}
