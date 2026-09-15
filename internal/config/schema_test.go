package config

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sort"
	"testing"
)

// The published schema is a second description of the same type, written
// by hand beside it. The conformance suite validates the files in this
// repository against the schema, which only catches a field the schema
// does not have once a file on disk carries it. Nothing caught a schema
// that had drifted from the type, so this does, in both directions.
func TestTheSchemaDescribesTheConfigurationType(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", "config.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}

	inSchema := schemaPaths(doc, "")
	if len(inSchema) == 0 {
		t.Fatal("the schema describes no properties, so this test checks nothing")
	}
	inType := typePaths()

	for path := range inType {
		if !inSchema[path] {
			t.Errorf("%s is in the configuration and the schema does not describe it", path)
		}
	}
	for path := range inSchema {
		if !inType[path] {
			t.Errorf("the schema describes %s, which the configuration does not have", path)
		}
	}
}

// Every object in the schema has to refuse what it does not name, or a
// misspelled setting would validate and then be dropped in silence.
func TestEveryObjectInTheSchemaRefusesWhatItDoesNotName(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "schemas", "config.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}

	var loose []string
	var walk func(node map[string]interface{}, path string)
	walk = func(node map[string]interface{}, path string) {
		props, ok := node["properties"].(map[string]interface{})
		if !ok {
			return
		}
		if extra, ok := node["additionalProperties"].(bool); !ok || extra {
			name := path
			if name == "" {
				name = "the file"
			}
			loose = append(loose, name)
		}
		for key, raw := range props {
			child, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			next := key
			if path != "" {
				next = path + "." + key
			}
			walk(child, next)
		}
	}
	walk(doc, "")

	sort.Strings(loose)
	if len(loose) > 0 {
		t.Errorf("these accept properties they do not name: %v", loose)
	}
}

// schemaPaths reports every property the schema describes, by the dotted
// name a person would type. An object with properties is walked; anything
// else is a leaf, which is how an array stops here.
func schemaPaths(node map[string]interface{}, prefix string) map[string]bool {
	out := map[string]bool{}
	props, ok := node["properties"].(map[string]interface{})
	if !ok {
		return out
	}
	for name, raw := range props {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		child, isObject := raw.(map[string]interface{})
		if !isObject {
			out[path] = true
			continue
		}
		nested := schemaPaths(child, path)
		if len(nested) == 0 {
			out[path] = true
			continue
		}
		for p := range nested {
			out[p] = true
		}
	}
	return out
}
