package schema

import (
	"github.com/invopop/jsonschema"

	"github.com/ps2build/ps2build/internal/config"
)

func Generate() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		FieldNameTag: "yaml",
	}
	return r.Reflect(&config.Config{})
}
