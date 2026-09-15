// Package jsonschema validates JSON against the part of JSON Schema that
// ZeroTurn's published schemas use.
//
// A full implementation is not needed here and a dependency is not wanted,
// so this supports the keywords the schemas in schemas/ actually use:
// type, required, properties, additionalProperties, items, enum, const,
// minimum, maximum, minLength, pattern for a small set of anchored
// patterns, oneOf, anyOf, and $ref to a definition in the same document.
// A schema that uses anything else is reported as unsupported rather than
// passed over in silence, so a contract can never appear to be checked
// when it is not.
package jsonschema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/output"
)

type Schema struct {
	doc  map[string]interface{}
	root map[string]interface{}
}

// Parse reads a schema document.
func Parse(b []byte) (*Schema, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("the schema is not valid JSON: %v", err)
	}
	return &Schema{doc: doc, root: doc}, nil
}

// Validate reports every problem it finds, in the order they appear, with
// the path of the value that failed.
func (s *Schema) Validate(b []byte) []string {
	var value interface{}
	if err := json.Unmarshal(b, &value); err != nil {
		return []string{fmt.Sprintf("the document is not valid JSON: %v", err)}
	}
	v := &validator{root: s.root}
	v.check("", s.doc, value)
	return v.problems
}

type validator struct {
	root     map[string]interface{}
	problems []string
}

func (v *validator) fail(path, format string, args ...interface{}) {
	if path == "" {
		path = "the document"
	}
	v.problems = append(v.problems, path+": "+fmt.Sprintf(format, args...))
}

var supported = map[string]bool{
	"$schema": true, "$id": true, "$ref": true, "$defs": true, "definitions": true,
	"title": true, "description": true, "examples": true, "default": true,
	"type": true, "required": true, "properties": true, "additionalProperties": true,
	"items": true, "enum": true, "const": true, "minimum": true, "maximum": true,
	"minLength": true, "minItems": true, "pattern": true, "oneOf": true, "anyOf": true,
}

func (v *validator) check(path string, schema map[string]interface{}, value interface{}) {
	for key := range schema {
		if !supported[key] {
			v.fail(path, "the schema uses %q, which this validator does not support", key)
		}
	}

	if ref, ok := schema["$ref"].(string); ok {
		target := v.resolve(ref)
		if target == nil {
			v.fail(path, "the reference %q could not be resolved", ref)
			return
		}
		v.check(path, target, value)
		return
	}

	if list, ok := schema["oneOf"].([]interface{}); ok {
		matches := 0
		for _, alt := range list {
			if m, ok := alt.(map[string]interface{}); ok && len(v.branch(path, m, value)) == 0 {
				matches++
			}
		}
		if matches != 1 {
			v.fail(path, "value matches %d of the alternatives in oneOf, it must match exactly one", matches)
		}
		return
	}
	if list, ok := schema["anyOf"].([]interface{}); ok {
		for _, alt := range list {
			if m, ok := alt.(map[string]interface{}); ok && len(v.branch(path, m, value)) == 0 {
				return
			}
		}
		v.fail(path, "value matches none of the alternatives in anyOf")
		return
	}

	if t, ok := schema["type"]; ok && !v.checkType(path, t, value) {
		return
	}
	if want, ok := schema["const"]; ok && !equal(want, value) {
		v.fail(path, "value must be %v", want)
	}
	if list, ok := schema["enum"].([]interface{}); ok {
		found := false
		for _, want := range list {
			if equal(want, value) {
				found = true
			}
		}
		if !found {
			v.fail(path, "value %v is not one of %v", value, list)
		}
	}

	switch value := value.(type) {
	case map[string]interface{}:
		v.checkObject(path, schema, value)
	case []interface{}:
		if items, ok := schema["items"].(map[string]interface{}); ok {
			for i, item := range value {
				v.check(fmt.Sprintf("%s[%d]", path, i), items, item)
			}
		}
		if min, ok := schema["minItems"].(float64); ok && float64(len(value)) < min {
			v.fail(path, "the list has %s, the schema requires at least %.0f",
				output.Counted(len(value), "1 entry", "%d entries"), min)
		}
	case string:
		if min, ok := schema["minLength"].(float64); ok && float64(len(value)) < min {
			v.fail(path, "the text is shorter than %.0f characters", min)
		}
		if pattern, ok := schema["pattern"].(string); ok {
			re, err := regexp.Compile(pattern)
			if err != nil {
				v.fail(path, "the schema pattern %q is not valid", pattern)
			} else if !re.MatchString(value) {
				v.fail(path, "value %q does not match %q", value, pattern)
			}
		}
	case float64:
		if min, ok := schema["minimum"].(float64); ok && value < min {
			v.fail(path, "value %v is below the minimum %v", value, min)
		}
		if max, ok := schema["maximum"].(float64); ok && value > max {
			v.fail(path, "value %v is above the maximum %v", value, max)
		}
	}
}

func (v *validator) checkObject(path string, schema map[string]interface{}, value map[string]interface{}) {
	props, _ := schema["properties"].(map[string]interface{})
	if list, ok := schema["required"].([]interface{}); ok {
		for _, name := range list {
			key, _ := name.(string)
			if _, present := value[key]; !present {
				v.fail(path, "the required property %q is missing", key)
			}
		}
	}
	names := make([]string, 0, len(value))
	for name := range value {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		child := join(path, name)
		if props != nil {
			if sub, ok := props[name].(map[string]interface{}); ok {
				v.check(child, sub, value[name])
				continue
			}
		}
		switch extra := schema["additionalProperties"].(type) {
		case bool:
			if !extra {
				v.fail(child, "this property is not part of the contract")
			}
		case map[string]interface{}:
			v.check(child, extra, value[name])
		}
	}
}

func (v *validator) checkType(path string, t interface{}, value interface{}) bool {
	switch want := t.(type) {
	case string:
		if matchesType(want, value) {
			return true
		}
		v.fail(path, "value is %s, the schema requires %s", typeName(value), want)
		return false
	case []interface{}:
		for _, alt := range want {
			if name, ok := alt.(string); ok && matchesType(name, value) {
				return true
			}
		}
		v.fail(path, "value is %s, the schema requires one of %v", typeName(value), want)
		return false
	}
	v.fail(path, "the schema type is not a string or a list")
	return false
}

func matchesType(want string, value interface{}) bool {
	switch want {
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		n, ok := value.(float64)
		return ok && n == math.Trunc(n)
	case "null":
		return value == nil
	}
	return false
}

func typeName(value interface{}) string {
	switch value.(type) {
	case map[string]interface{}:
		return "an object"
	case []interface{}:
		return "a list"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case float64:
		return "a number"
	case nil:
		return "null"
	}
	return "an unknown kind of value"
}

func (v *validator) resolve(ref string) map[string]interface{} {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	node := interface{}(v.root)
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		m, ok := node.(map[string]interface{})
		if !ok {
			return nil
		}
		node = m[part]
	}
	m, _ := node.(map[string]interface{})
	return m
}

// branch validates against an alternative without recording its problems.
func (v *validator) branch(path string, schema map[string]interface{}, value interface{}) []string {
	sub := &validator{root: v.root}
	sub.check(path, schema, value)
	return sub.problems
}

func equal(a, b interface{}) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ab) == string(bb)
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// Unsupported reports every keyword in the schema that this validator
// does not implement, wherever it appears.
//
// Validate only reaches the parts of a schema that the document it is
// given exercises, so checking one empty document screened the root and
// nothing else. A keyword on a field no sample happens to carry would
// then be accepted in silence, and a contract would look checked when it
// was not.
func (s *Schema) Unsupported() []string {
	var out []string
	seen := map[string]bool{}
	add := func(path, key string) {
		msg := path + ": " + key
		if seen[msg] {
			return
		}
		seen[msg] = true
		out = append(out, msg)
	}

	var node func(path string, n map[string]interface{})
	var value func(path string, v interface{})

	value = func(path string, v interface{}) {
		if m, ok := v.(map[string]interface{}); ok {
			node(path, m)
		}
	}

	node = func(path string, n map[string]interface{}) {
		for key, raw := range n {
			if !supported[key] {
				add(path, key)
				continue
			}
			child := path + "/" + key
			switch key {
			// A map of names to schemas. The names are not keywords.
			case "properties", "$defs", "definitions":
				if m, ok := raw.(map[string]interface{}); ok {
					for name, sub := range m {
						value(child+"/"+name, sub)
					}
				}
			// A schema, or in the case of additionalProperties a bool.
			case "items", "additionalProperties":
				value(child, raw)
			// A list of schemas.
			case "oneOf", "anyOf":
				if list, ok := raw.([]interface{}); ok {
					for i, sub := range list {
						value(fmt.Sprintf("%s/%d", child, i), sub)
					}
				}
			}
			// Everything else is a value rather than a schema, so it is
			// not walked: enum members and examples are documents.
		}
	}

	node("", s.doc)
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
