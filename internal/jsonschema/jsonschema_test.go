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

// oneOf means exactly one, so a value matching two alternatives is as
// wrong as one matching none. A published contract using it says the
// shapes are alternatives rather than a list to pick from.
func TestOneOfRefusesAValueThatMatchesTwoAlternatives(t *testing.T) {
	s := parse(t, `{"oneOf":[{"type":"string"},{"type":"string","minLength":1}]}`)
	p := s.Validate([]byte(`"ab"`))
	if len(p) == 0 {
		t.Fatal("a value matching both alternatives was accepted")
	}
	if !strings.Contains(p[0], "exactly one") {
		t.Errorf("the problem does not say what the rule is: %v", p)
	}

	// Three alternatives, one of which matches. Two alternatives cannot
	// tell counting the matches from counting the failures, because one
	// of each is the same number.
	three := parse(t, `{"oneOf":[{"type":"string"},{"type":"number"},{"type":"boolean"}]}`)
	if p := three.Validate([]byte(`"a"`)); len(p) != 0 {
		t.Errorf("a value matching one of three alternatives was refused: %v", p)
	}
}

// anyOf means at least one, which is the rule oneOf is not.
func TestAnyOfTakesAValueMatchingMoreThanOne(t *testing.T) {
	s := parse(t, `{"anyOf":[{"type":"string"},{"type":"string","minLength":1}]}`)
	if p := s.Validate([]byte(`"ab"`)); len(p) != 0 {
		t.Fatalf("a value matching both alternatives was refused: %v", p)
	}
	if p := s.Validate([]byte(`5`)); len(p) == 0 {
		t.Fatal("a value matching neither alternative was accepted")
	}
}

// Every bound in a schema is inclusive, so the value sitting exactly on
// it is the one that says which way the comparison goes.
func TestABoundIsMetExactly(t *testing.T) {
	list := parse(t, `{"type":"array","minItems":2,"items":{"type":"integer"}}`)
	if p := list.Validate([]byte(`[1,2]`)); len(p) != 0 {
		t.Errorf("a list of exactly the minimum was refused: %v", p)
	}
	if p := list.Validate([]byte(`[1]`)); len(p) == 0 {
		t.Error("a list shorter than the minimum was accepted")
	}

	text := parse(t, `{"type":"string","minLength":2}`)
	if p := text.Validate([]byte(`"ab"`)); len(p) != 0 {
		t.Errorf("text of exactly the minimum length was refused: %v", p)
	}
	if p := text.Validate([]byte(`"a"`)); len(p) == 0 {
		t.Error("text shorter than the minimum was accepted")
	}
}

// A problem names the path it was found at, because a reader given only
// the message has to search the document for it.
func TestAProblemNamesWhereItIs(t *testing.T) {
	s := parse(t, `{"type":"object","properties":{"outer":{"type":"object",
		"properties":{"inner":{"type":"string"}}}}}`)
	p := s.Validate([]byte(`{"outer":{"inner":5}}`))
	if len(p) == 0 {
		t.Fatal("a wrong type inside an object was accepted")
	}
	if !strings.Contains(p[0], "outer.inner") {
		t.Errorf("the problem does not name the path: %v", p)
	}
}
