package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ps2build/ps2build/internal/utils"
	"gopkg.in/yaml.v3"

	"github.com/ps2build/ps2build/internal/pkgmanifest"
)

type meta struct {
	Tier    string `yaml:"tier"`
	License string `yaml:"license"`
}

func readMeta(packageYAMLPath string) meta {
	data, err := os.ReadFile(packageYAMLPath)
	if err != nil {
		return meta{}
	}
	var m meta
	_ = yaml.Unmarshal(data, &m)
	return m
}

// installedPackage pairs one package.yaml's real pkgmanifest.Manifest
// (the build-relevant fields) with its meta (the disclosure-only fields).
type installedPackage struct {
	path string
	tier string
	m    *pkgmanifest.Manifest
	meta meta
}

func listInstalled(packageRoot string) ([]installedPackage, error) {
	var out []installedPackage
	for _, tier := range []string{"core", "world"} {
		tierDir := filepath.Join(packageRoot, tier)
		entries, err := os.ReadDir(tierDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(tierDir, e.Name(), "package.yaml")
			if _, err := os.Stat(p); err != nil {
				continue
			}
			m, err := pkgmanifest.Load(p)
			if err != nil {
				continue
			}
			out = append(out, installedPackage{
				path: p,
				tier: tier,
				m:    m,
				meta: readMeta(p),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].m.Name < out[j].m.Name })
	return out, nil
}

func findQuiet(name string) (*pkgmanifest.Manifest, bool, error) {
	return pkgmanifest.Find(utils.PackageRoot(), name)
}

func artifactKinds(m *pkgmanifest.Manifest) []string {
	seen := map[string]bool{}
	var kinds []string
	for _, a := range m.Artifacts {
		if !seen[a.Kind] {
			seen[a.Kind] = true
			kinds = append(kinds, a.Kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}

// --- list_packages ---

type listPackagesArgs struct {
	Tier         string `json:"tier,omitempty" jsonschema:"Only return packages in this tier: 'core' or 'world'. Omit for both."`
	OriginPrefix string `json:"origin_prefix,omitempty" jsonschema:"Only return packages whose origin starts with this (e.g. 'port:' for wrapped third-party ports, 'ps2sdk' for the SDK's own modules)."`
}

type packageSummary struct {
	Name          string   `json:"name"`
	Tier          string   `json:"tier"`
	Origin        string   `json:"origin"`
	License       string   `json:"license,omitempty"`
	ArtifactKinds []string `json:"artifact_kinds"`
}

type listPackagesResult struct {
	Packages []packageSummary `json:"packages"`
	Count    int              `json:"count"`
}

func registerListPackages(s *mcp.Server, packageRoot string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_packages",
		Description: "List every installed package.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listPackagesArgs) (*mcp.CallToolResult, listPackagesResult, error) {
		pkgs, err := listInstalled(packageRoot)
		if err != nil {
			return nil, listPackagesResult{}, err
		}

		var out []packageSummary
		for _, p := range pkgs {
			if args.Tier != "" && p.tier != args.Tier {
				continue
			}
			if args.OriginPrefix != "" && !strings.HasPrefix(p.m.Origin, args.OriginPrefix) {
				continue
			}
			out = append(out, packageSummary{
				Name:          p.m.Name,
				Tier:          p.tier,
				Origin:        p.m.Origin,
				License:       p.meta.License,
				ArtifactKinds: artifactKinds(p.m),
			})
		}
		return nil, listPackagesResult{Packages: out, Count: len(out)}, nil
	})
}

// --- describe_package ---

type describePackageArgs struct {
	Name string `json:"name" jsonschema:"The package's directory name under packages/{core,world}/ - the same string a libs:/headers:/embed_irx: entry referencing it would use."`
}

type artifactDetail struct {
	Kind           string   `json:"kind"`
	Target         []string `json:"target"`
	IncludeDirs    []string `json:"include_dirs,omitempty"`
	Lib            string   `json:"lib,omitempty"`
	IRX            string   `json:"irx,omitempty"`
	Headers        []string `json:"headers,omitempty"`
	Links          []string `json:"links,omitempty"`
	Deprecated     string   `json:"deprecated,omitempty"`
	NamingMismatch string   `json:"naming_mismatch,omitempty"`
}

type describePackageResult struct {
	Name      string           `json:"name"`
	Tier      string           `json:"tier"`
	Origin    string           `json:"origin"`
	License   string           `json:"license,omitempty"`
	Path      string           `json:"path"`
	Artifacts []artifactDetail `json:"artifacts"`
}

func registerDescribePackage(s *mcp.Server, packageRoot string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "describe_package",
		Description: "Full manifest detail for one installed package: its artifacts, resolved paths, and declared headers:/links: dependencies.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args describePackageArgs) (*mcp.CallToolResult, describePackageResult, error) {
		m, ok, err := pkgmanifest.Find(packageRoot, args.Name)
		if err != nil {
			return nil, describePackageResult{}, err
		}
		if !ok {
			return nil, describePackageResult{}, fmt.Errorf("no package named %q under %s/{core,world}", args.Name, packageRoot)
		}

		tier := "core"
		yamlPath := packageYAMLPathFor(packageRoot, "core", args.Name)
		if _, err := os.Stat(yamlPath); err != nil {
			tier = "world"
			yamlPath = packageYAMLPathFor(packageRoot, "world", args.Name)
		}
		meta := readMeta(yamlPath)

		var artifacts []artifactDetail
		for _, a := range m.Artifacts {
			d := artifactDetail{
				Kind:        a.Kind,
				Target:      a.Target,
				IncludeDirs: m.AbsIncludeDirs(a),
				Headers:     a.Headers,
				Links:       a.Links,
				Deprecated:  a.Deprecated,
			}
			if a.Lib != "" {
				d.Lib = m.AbsLib(a)
				base := filepath.Base(a.Lib)
				want := "lib" + args.Name + ".a"
				if base != want {
					d.NamingMismatch = "archive is " + base + ", but a consumer's libs: [" + args.Name + "] entry produces -l" + args.Name + ", which the linker resolves to " + want + " - these don't match, so linking will fail with \"cannot find -l" + args.Name + "\" even though this package resolves fine"
				}
			}
			if a.IRX != "" {
				d.IRX = m.AbsIRX(a)
			}
			artifacts = append(artifacts, d)
		}

		return nil, describePackageResult{
			Name:      m.Name,
			Tier:      tier,
			Origin:    m.Origin,
			License:   meta.License,
			Path:      filepath.Join(packageRoot, tier, args.Name, "package.yaml"),
			Artifacts: artifacts,
		}, nil
	})
}

func packageYAMLPathFor(packageRoot, tier, name string) string {
	return filepath.Join(packageRoot, tier, name, "package.yaml")
}

// --- search_packages ---

type searchPackagesArgs struct {
	Query string `json:"query" jsonschema:"Free-text query, matched case-insensitively against package name, origin, and license."`
}

func registerSearchPackages(s *mcp.Server, packageRoot string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_packages",
		Description: "Search installed packages by name/origin/license substring - use before reaching for a new port or vendoring source that might already be installed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchPackagesArgs) (*mcp.CallToolResult, listPackagesResult, error) {
		pkgs, err := listInstalled(packageRoot)
		if err != nil {
			return nil, listPackagesResult{}, err
		}
		q := strings.ToLower(args.Query)

		var out []packageSummary
		for _, p := range pkgs {
			hay := strings.ToLower(p.m.Name + " " + p.m.Origin + " " + p.meta.License)
			if !strings.Contains(hay, q) {
				continue
			}
			out = append(out, packageSummary{
				Name:          p.m.Name,
				Tier:          p.tier,
				Origin:        p.m.Origin,
				License:       p.meta.License,
				ArtifactKinds: artifactKinds(p.m),
			})
		}
		return nil, listPackagesResult{Packages: out, Count: len(out)}, nil
	})
}
