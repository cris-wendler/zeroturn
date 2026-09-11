package jsonschema

import (
	"strings"
	"testing"
)

func parse(t *testing.T, schema string) *Schema {
	t.Helper()
	s, err := Parse([]byte(schema))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTypesAndRequired(t *testing.T) {
	s := parse(t, `{"type":"object","required":["name","count"],
		"properties":{"name":{"type":"string"},"count":{"type":"integer"},"on":{"type":"boolean"}},
		"additionalProperties":false}`)
	if p := s.Validate([]byte(`{"name":"a","count":2,"on":true}`)); len(p) != 0 {
		t.Fatalf("valid document reported %v", p)
	}
	cases := map[string]string{
		`{"name":"a"}`:                     "required property",
		`{"name":5,"count":2}`:             "requires string",
		`{"name":"a","count":2.5}`:         "requires integer",
		`{"name":"a","count":2,"extra":1}`: "not part of the contract",
		`[]`:                               "requires object",
	}
	for doc, want := range cases {
		p := s.Validate([]byte(doc))
		if len(p) == 0 || !strings.Contains(strings.Join(p, " "), want) {
			t.Errorf("%s: problems %v, want one mentioning %q", doc, p, want)
		}
	}
}

func TestNullableAndEnum(t *testing.T) {
	s := parse(t, `{"type":"object","properties":{
		"pct":{"type":["number","null"],"minimum":0,"maximum":100},
		"decision":{"enum":["allow","ask","deny"]}}}`)
	for _, ok := range []string{`{"pct":null}`, `{"pct":0}`, `{"pct":100}`, `{"decision":"ask"}`} {
		if p := s.Validate([]byte(ok)); len(p) != 0 {
			t.Errorf("%s: %v", ok, p)
		}
	}
	for _, bad := range []string{`{"pct":101}`, `{"pct":-1}`, `{"pct":"80"}`, `{"decision":"maybe"}`} {
		if p := s.Validate([]byte(bad)); len(p) == 0 {
			t.Errorf("%s was accepted", bad)
		}
	}
}

func TestArraysAndReferences(t *testing.T) {
	s := parse(t, `{"type":"object","properties":{"steps":{"type":"array","items":{"$ref":"#/$defs/step"}}},
		"$defs":{"step":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}},"additionalProperties":false}}}`)
	if p := s.Validate([]byte(`{"steps":[{"name":"lint"},{"name":"test"}]}`)); len(p) != 0 {
		t.Fatalf("%v", p)
	}
	p := s.Validate([]byte(`{"steps":[{"name":"lint"},{"nope":1}]}`))
	if len(p) == 0 || !strings.Contains(p[0], "steps[1]") {
		t.Fatalf("problems %v, want one naming steps[1]", p)
	}
}

func TestPatternAndConst(t *testing.T) {
	s := parse(t, `{"type":"object","properties":{
		"contract":{"const":"zeroturn.event/1"},
		"version":{"type":"string","pattern":"^[0-9]+\\.[0-9]+\\.[0-9]+$"}}}`)
	if p := s.Validate([]byte(`{"contract":"zeroturn.event/1","version":"1.0.0"}`)); len(p) != 0 {
		t.Fatalf("%v", p)
	}
	if p := s.Validate([]byte(`{"contract":"other/2"}`)); len(p) == 0 {
		t.Fatal("a wrong contract was accepted")
	}
	if p := s.Validate([]byte(`{"version":"1.0"}`)); len(p) == 0 {
		t.Fatal("a version that does not match the pattern was accepted")
	}
}

func TestOneOf(t *testing.T) {
	s := parse(t, `{"oneOf":[{"type":"string"},{"type":"number"}]}`)
	if p := s.Validate([]byte(`"a"`)); len(p) != 0 {
		t.Fatalf("%v", p)
	}
	if p := s.Validate([]byte(`true`)); len(p) == 0 {
		t.Fatal("a boolean matched neither alternative but was accepted")
	}
}

// An unsupported keyword must be reported. A contract that looks checked
// but is not would be worse than no check at all.
func TestUnsupportedKeywordIsReported(t *testing.T) {
	s := parse(t, `{"type":"object","patternProperties":{"^x":{"type":"string"}}}`)
	p := s.Validate([]byte(`{}`))
	if len(p) == 0 || !strings.Contains(p[0], "does not support") {
		t.Fatalf("problems %v", p)
	}
}

func TestInvalidDocuments(t *testing.T) {
	if _, err := Parse([]byte(`{not json`)); err == nil {
		t.Fatal("an invalid schema was accepted")
	}
	s := parse(t, `{"type":"object"}`)
	if p := s.Validate([]byte(`{not json`)); len(p) == 0 {
		t.Fatal("an invalid document was accepted")
	}
}
