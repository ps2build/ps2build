package config

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the highest schema_version this ps2build reads, in ps2.yaml and ps2workspace.yaml alike.
const SchemaVersion = 1

// BuildVersion is ps2build's own version, named when a file needs a newer one. The cli sets it at start-up.
var BuildVersion = "dev"

// CheckSchemaVersion rejects a schema_version this ps2build cannot read. A missing one (nil) means 1.
func CheckSchemaVersion(file string, v *int) error {
	switch {
	case v == nil || (*v >= 1 && *v <= SchemaVersion):
		return nil
	case *v < 1:
		return fmt.Errorf("%s: schema_version must be 1 or higher, got %d", file, *v)
	default:
		return fmt.Errorf("%s: schema_version %d needs a newer ps2build (this is %s, which reads up to %d); run \"ps2build update\"", file, *v, BuildVersion, SchemaVersion)
	}
}

// Decode parses the YAML in data into out (a pointer to a struct) and returns one message for every key out has no
// field for; those keys are ignored. path only names the file in messages. An empty document is io.EOF.
func Decode(path string, data []byte, out any) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Kind == 0 {
		return nil, fmt.Errorf("parsing %s: %w", path, io.EOF)
	}
	if err := doc.Decode(out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	var warnings []string
	walkKeys(&doc, reflect.TypeOf(out), "", func(key string) {
		warnings = append(warnings, fmt.Sprintf("%s: unknown key %q ignored", path, key))
	})
	return warnings, nil
}

var (
	warnedMu sync.Mutex
	warned   = map[string]bool{}
)

// Warn prints each warning on stderr, once per process however many times the file is loaded.
func Warn(warnings []string) {
	warnedMu.Lock()
	defer warnedMu.Unlock()
	for _, w := range warnings {
		if warned[w] {
			continue
		}
		warned[w] = true
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
}

func walkKeys(n *yaml.Node, t reflect.Type, path string, unknown func(string)) {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) > 0 {
			walkKeys(n.Content[0], t, path, unknown)
		}
	case yaml.SequenceNode:
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return
		}
		for i, c := range n.Content {
			walkKeys(c, t.Elem(), fmt.Sprintf("%s[%d]", path, i), unknown)
		}
	case yaml.MappingNode:
		switch t.Kind() {
		case reflect.Struct:
			fields := yamlFields(t)
			for _, kv := range mappingPairs(n) {
				name := kv[0].Value
				child := join(path, name)
				f, ok := fields[name]
				if !ok {
					unknown(child)
					continue
				}
				walkKeys(kv[1], f, child, unknown)
			}
		case reflect.Map:
			for _, kv := range mappingPairs(n) {
				walkKeys(kv[1], t.Elem(), join(path, kv[0].Value), unknown)
			}
		}
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// mappingPairs lists a mapping's key/value nodes, folding in the entries of any "<<" merge keys.
func mappingPairs(n *yaml.Node) [][2]*yaml.Node {
	var out [][2]*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Tag != "!!merge" {
			out = append(out, [2]*yaml.Node{k, v})
			continue
		}
		for v.Kind == yaml.AliasNode {
			v = v.Alias
		}
		switch v.Kind {
		case yaml.MappingNode:
			out = append(out, mappingPairs(v)...)
		case yaml.SequenceNode:
			for _, m := range v.Content {
				for m.Kind == yaml.AliasNode {
					m = m.Alias
				}
				if m.Kind == yaml.MappingNode {
					out = append(out, mappingPairs(m)...)
				}
			}
		}
	}
	return out
}

// yamlFields maps each yaml key of struct type t to the type of its field.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if strings.Contains(","+opts+",", ",inline,") {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range yamlFields(ft) {
					fields[k] = v
				}
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		fields[name] = f.Type
	}
	return fields
}
