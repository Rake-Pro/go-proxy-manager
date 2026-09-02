package model

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnknownYAMLKeys parses data as YAML and reports every mapping key that has no
// corresponding field on out's type, checked recursively through nested structs
// (including ",inline" embedded fields, e.g. every object's embedded
// ObjectMeta) and slices of structs.
//
// A map-typed field - map[string]string, a free-form map, or a map of structs
// such as Settings.IngressDiscovery.Profiles - is opaque: its contents are data,
// not schema, so nothing under it is ever checked. Same for a scalar leaf
// (string, bool, number, time.Time, a Secret) - there is nothing under it to
// walk.
//
// This exists because both config loaders use a non-strict yaml.Unmarshal (see
// docs/operations/upgrading.md#rollback): a key an older struct does not know
// about is silently dropped rather than rejected. UnknownYAMLKeys lets a loader
// that just succeeded warn about what it silently ignored, without changing
// what it accepts.
//
// Paths use dotted/indexed notation, e.g. "upstream.badKey" or
// "locations[0].rewrite.notARealField". A YAML parse failure yields a nil
// slice: the caller's own yaml.Unmarshal into the real struct already reports
// that failure, and this function only has something to say about a load that
// otherwise succeeded.
func UnknownYAMLKeys(data []byte, out any) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	t := reflect.TypeOf(out)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		return nil
	}
	var found []string
	walkUnknownKeys(doc.Content[0], t, "", &found)
	return found
}

// walkUnknownKeys compares one YAML mapping node against the yaml-tagged
// fields of t and recurses into nested structs and slices of structs. Anything
// else (t not a struct, node not a mapping) has nothing to compare and returns
// without effect.
func walkUnknownKeys(node *yaml.Node, t reflect.Type, path string, found *[]string) {
	if node == nil || node.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return
	}
	fields := yamlFieldSet(t)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]
		f, ok := fields[key]
		if !ok {
			*found = append(*found, joinYAMLPath(path, key))
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		childPath := joinYAMLPath(path, key)
		switch ft.Kind() {
		case reflect.Struct:
			walkUnknownKeys(val, ft, childPath, found)
		case reflect.Slice, reflect.Array:
			et := ft.Elem()
			for et.Kind() == reflect.Ptr {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct && val.Kind == yaml.SequenceNode {
				for idx, item := range val.Content {
					walkUnknownKeys(item, et, fmt.Sprintf("%s[%d]", childPath, idx), found)
				}
			}
		}
		// reflect.Map and every scalar kind are opaque/leaf: nothing further to
		// check under them.
	}
}

// yamlFieldSet returns t's YAML wire keys mapped to the struct field that
// serializes them, flattening any field embedded with a ",inline" (or bare
// anonymous) yaml tag into the same level - which is how ObjectMeta reaches
// every first-class object's top level. Unexported, non-embedded fields and
// fields tagged yaml:"-" are omitted, matching what yaml.v3 itself would never
// read or write.
func yamlFieldSet(t reflect.Type) map[string]reflect.StructField {
	out := map[string]reflect.StructField{}
	if t.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue // unexported, not embedded: yaml.v3 never touches it
		}
		name, opts := parseYAMLTag(f.Tag.Get("yaml"))
		if name == "-" {
			continue
		}
		inline := false
		for _, o := range opts {
			if o == "inline" {
				inline = true
			}
		}
		if inline || (f.Anonymous && name == "") {
			ft := f.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range yamlFieldSet(ft) {
					out[k] = v
				}
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		out[name] = f
	}
	return out
}

// parseYAMLTag splits a yaml struct tag ("name,opt1,opt2") into its name and
// options, the same shape gopkg.in/yaml.v3 parses internally.
func parseYAMLTag(tag string) (name string, opts []string) {
	if tag == "" {
		return "", nil
	}
	parts := strings.Split(tag, ",")
	return parts[0], parts[1:]
}

// joinYAMLPath appends key to parent with a "." separator, or returns key
// alone at the root.
func joinYAMLPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}
