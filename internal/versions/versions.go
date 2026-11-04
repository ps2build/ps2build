package versions

import (
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"
)

// File is one versions.yaml.
type File struct {
	Suite      string            `yaml:"suite"`
	Components map[string]string `yaml:"components"`
}

func Load(ps2dev string) (File, error) {
	p := path.Join(ps2dev, "versions.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	return f, nil
}

// Save writes f to <ps2dev>/versions.yaml verbatim, replacing whatever
// was there.
func Save(ps2dev string, f File) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(ps2dev, "versions.yaml"), data, 0o644)
}

func SaveComponent(ps2dev, name, tag string) error {
	f, err := Load(ps2dev)
	if err != nil {
		f = File{}
	}
	if f.Components == nil {
		f.Components = map[string]string{}
	}
	f.Components[name] = tag
	return Save(ps2dev, f)
}
