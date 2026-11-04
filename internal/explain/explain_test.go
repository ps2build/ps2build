package explain

import (
	"strings"
	"testing"
)

func TestRenderText_TargetsAliasesTargetTypes(t *testing.T) {
	doc := BuildDoc()

	got, err := field(doc, "targets")
	if err != nil {
		t.Fatalf(`field(doc, "targets") error = %v, want nil - "targets" should alias "target_types"`, err)
	}
	want, err := field(doc, "target_types")
	if err != nil {
		t.Fatalf(`field(doc, "target_types") error = %v`, err)
	}
	if got != want {
		t.Errorf(`field(doc, "targets") = %q, want it to match field(doc, "target_types") = %q`, got, want)
	}
}

func TestRenderText_UnknownSectionListsValidOnes(t *testing.T) {
	doc := BuildDoc()
	_, err := RenderText(doc, "bogus")
	if err == nil {
		t.Fatal("RenderText(doc, \"bogus\") error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "target_types") {
		t.Errorf("error %q should list target_types among the valid sections", err)
	}
}

func TestValidSections_NoAliasesListed(t *testing.T) {
	sections := ValidSections()
	count := 0
	for _, s := range sections {
		if s == "target_types" {
			count++
		}
		if s == "targets" {
			t.Errorf("ValidSections() contains %q - the alias must not be listed, only accepted", s)
		}
	}
	if count != 1 {
		t.Errorf("ValidSections() contains target_types %d times, want exactly 1", count)
	}
}
