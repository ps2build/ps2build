package pkgmanifest

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

type Artifact struct {
	Kind        string   `yaml:"kind"`
	Target      []string `yaml:"target"`
	IncludeDirs []string `yaml:"include_dirs"`
	Headers     []string `yaml:"headers"`
	Links       []string `yaml:"links"`
	Lib         string   `yaml:"lib"`
	IRX         string   `yaml:"irx"`
	CXX         bool     `yaml:"cxx"`
	Deprecated  string   `yaml:"deprecated,omitempty"`
	Linkfile    string   `yaml:"linkfile,omitempty"`
}

// Manifest is one package.yaml.
type Manifest struct {
	Name      string     `yaml:"name"`
	Origin    string     `yaml:"origin"`
	Artifacts []Artifact `yaml:"artifacts"`

	dir string
}

// LibraryArtifact returns the artifact of kind "library" whose Target list
// contains target ("ee" or "iop"), if any.
func (m *Manifest) LibraryArtifact(target string) (Artifact, bool) {
	return m.artifactOfKind("library", target)
}

func (m *Manifest) DriverArtifact(target string) (Artifact, bool) {
	return m.artifactOfKind("driver", target)
}

func (m *Manifest) HeadersArtifact(target string) (Artifact, bool) {
	return m.artifactOfKind("headers", target)
}

func (m *Manifest) artifactOfKind(kind, target string) (Artifact, bool) {
	for _, a := range m.Artifacts {
		if a.Kind != kind {
			continue
		}
		if slices.Contains(a.Target, target) {
			return a, true
		}
	}
	return Artifact{}, false
}

func (m *Manifest) AbsIncludeDirs(a Artifact) []string {
	out := make([]string, len(a.IncludeDirs))
	for i, inc := range a.IncludeDirs {
		out[i] = path.Join(m.dir, inc)
	}
	return out
}

// AbsLib resolves an artifact's Lib path against the manifest's own
// directory.
func (m *Manifest) AbsLib(a Artifact) string {
	return path.Join(m.dir, a.Lib)
}

func (m *Manifest) AbsLinkfile(a Artifact) string {
	return path.Join(m.dir, a.Linkfile)
}

// AbsIRX resolves a driver artifact's IRX path against the manifest's own
// directory.
func (m *Manifest) AbsIRX(a Artifact) string {
	return path.Join(m.dir, a.IRX)
}

// Load parses one package.yaml at path.
func Load(manifestPath string) (*Manifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", manifestPath, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("%s: missing required field \"name\"", manifestPath)
	}
	m.dir = filepath.ToSlash(filepath.Dir(manifestPath))
	return &m, nil
}

func Find(packageRoot, name string) (*Manifest, bool, error) {
	for _, tier := range []string{"core", "world"} {
		p := path.Join(packageRoot, tier, name, "package.yaml")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		m, err := Load(p)
		if err != nil {
			return nil, false, err
		}
		return m, true, nil
	}
	return nil, false, nil
}
