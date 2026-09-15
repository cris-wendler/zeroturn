package config

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// leaves reads every single value out of the type itself, by the JSON
// path a person would type, so the tests below compare the registry
// against the shape rather than against a list someone maintains. A list
// is not a leaf: the registry describes values that can be set by name,
// and verify steps are not one of those.
func leaves(c Config) map[string]string {
	out := map[string]string{}
	var walk func(v reflect.Value, prefix string)
	walk = func(v reflect.Value, prefix string) {
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Struct:
				walk(f, path)
			case reflect.Int:
				out[path] = strconv.FormatInt(f.Int(), 10)
			case reflect.String:
				out[path] = f.String()
			case reflect.Slice:
				// Not a setting. A list is edited in the file or by the
				// command that owns it, not by name with one value.
			default:
				out[path] = "unsupported kind " + f.Kind().String()
			}
		}
	}
	walk(reflect.ValueOf(c), "")
	return out
}

// notSettings are the values in the file that no key describes, each with
// the reason. Anything else that is a single value has to be reachable by
// name, or somebody can read it in the file and find no way to change it.
var notSettings = map[string]string{
	"version": "the file format marker, which migration owns",
}

// The registry has to cover the type in both directions. A setting added
// with no key would be unreachable by name, and a key naming a field that
// is gone would offer something that cannot be set.
func TestEverySettingHasAKey(t *testing.T) {
	inType := leaves(Default())
	inRegistry := map[string]bool{}
	for _, name := range Names() {
		inRegistry[name] = true
	}
	for path := range inType {
		if inRegistry[path] || notSettings[path] != "" {
			continue
		}
		t.Errorf("%s is a setting with no key, so nobody can change it by name", path)
	}
	for name := range inRegistry {
		if _, ok := inType[name]; !ok {
			t.Errorf("the registry offers %s, which is not a field of the configuration", name)
		}
	}
	// An exception that no longer names anything real would hide the next
	// setting that needs one.
	for path := range notSettings {
		if _, ok := inType[path]; !ok {
			t.Errorf("%s is excused from having a key and is not in the configuration", path)
		}
	}
}

// A key holds an accessor, and an accessor can point at the wrong field
// without failing to compile. Writing through each key must change the
// one setting it names and nothing else.
func TestEachKeyWritesTheFieldItNames(t *testing.T) {
	for _, k := range Keys() {
		g := Default()
		before := leaves(g)

		var next string
		switch k.Kind {
		case KindChoice:
			for _, c := range k.Choices {
				if c != before[k.Name] {
					next = c
					break
				}
			}
		case KindText:
			next = before[k.Name] + "-other"
		default:
			n, err := strconv.Atoi(before[k.Name])
			if err != nil {
				t.Errorf("%s: current value %q is not a number", k.Name, before[k.Name])
				continue
			}
			next = strconv.Itoa(n + 1)
		}
		if next == "" {
			t.Errorf("%s: no second value to write", k.Name)
			continue
		}

		if err := k.Set(&g, next); err != nil {
			t.Errorf("%s: writing %q: %v", k.Name, next, err)
			continue
		}
		var changed []string
		for path, was := range before {
			if leaves(g)[path] != was {
				changed = append(changed, path)
			}
		}
		if len(changed) != 1 || changed[0] != k.Name {
			t.Errorf("%s: writing it changed %v", k.Name, changed)
			continue
		}
		if got := k.Value(g); got != next {
			t.Errorf("%s: wrote %q, reads back %q", k.Name, next, got)
		}
	}
}

// The wording that refuses a value is part of what a person sees, and it
// was written out by hand at every key before the registry existed.
func TestRefusingAValueSaysWhatIsAccepted(t *testing.T) {
	cases := map[string]struct{ value, detail, fix string }{
		"guard.mode": {"sideways", "is not a guard mode",
			"use observe, confirm, or strict"},
		"guard.limits.projection": {"maybe", "is not a projection setting",
			"use on, or off"},
		"guard.credentials.mode": {"loud", "is not a credential guard mode",
			"use off, ask, or deny"},
		"guard.credentials.prompts": {"sometimes", "is not a prompt guard setting",
			"use off, or block"},
		"guard.context.warn": {"most of it", "is not a whole number",
			"supply a whole number"},
	}
	for name, c := range cases {
		k, ok := Lookup(name)
		if !ok {
			t.Errorf("%s is not in the registry", name)
			continue
		}
		g := Default()
		err := k.Set(&g, c.value)
		ve, isValidation := err.(ValidationError)
		if !isValidation {
			t.Errorf("%s: got %v, want a ValidationError", name, err)
			continue
		}
		if ve.Field != name {
			t.Errorf("%s: error names %q", name, ve.Field)
		}
		if !strings.Contains(ve.Detail, c.detail) {
			t.Errorf("%s: detail %q", name, ve.Detail)
		}
		if ve.Fix != c.fix {
			t.Errorf("%s: fix %q, want %q", name, ve.Fix, c.fix)
		}
		// A refused value leaves the setting alone.
		if k.Value(g) != k.Value(Default()) {
			t.Errorf("%s: a refused value was written anyway", name)
		}
	}
}

// A choice is accepted whatever case it is typed in, which is what the
// command did before the registry.
func TestAChoiceIsReadWithoutRegardToCase(t *testing.T) {
	k, _ := Lookup("guard.mode")
	g := Default()
	if err := k.Set(&g, "STRICT"); err != nil {
		t.Fatal(err)
	}
	if g.Guard.Mode != ModeStrict {
		t.Fatalf("mode %q", g.Guard.Mode)
	}
}

// A bare number in the table says nothing about what it counts.
func TestDisplayCarriesTheUnit(t *testing.T) {
	g := Default()
	want := map[string]string{
		"guard.context.warn":                strconv.Itoa(g.Guard.Context.Warn) + "%",
		"guard.session.durationWarnMinutes": strconv.Itoa(g.Guard.Session.DurationWarnMinutes) + " minutes",
		"guard.session.activeSubagentsWarn": strconv.Itoa(g.Guard.Session.ActiveSubagentsWarn),
		"guard.mode":                        g.Guard.Mode,
	}
	for name, w := range want {
		k, ok := Lookup(name)
		if !ok {
			t.Errorf("%s is not in the registry", name)
			continue
		}
		if got := k.Display(g); got != w {
			t.Errorf("%s displays %q, want %q", name, got, w)
		}
	}
}

func TestLookupRefusesAKeyThatDoesNotExist(t *testing.T) {
	if _, ok := Lookup("guard.context.panic"); ok {
		t.Fatal("a key nobody defined was found")
	}
}

// Every percentage is range checked, and the check reads the registry,
// so a percentage setting added later is covered without being added to
// a second list.
func TestEveryPercentKeyIsRangeChecked(t *testing.T) {
	for _, k := range Keys() {
		if k.Kind != KindPercent {
			continue
		}
		c := Default()
		if err := k.Set(&c, "0"); err != nil {
			t.Fatal(err)
		}
		err := c.Validate()
		ve, isValidation := err.(ValidationError)
		if !isValidation {
			t.Errorf("%s at 0 gave %v, want a ValidationError", k.Name, err)
			continue
		}
		if ve.Field != k.Name {
			// Another rule may fire first, such as warn having to stay
			// below confirm. What matters is that something refused it.
			if !strings.HasPrefix(ve.Field, "guard.") {
				t.Errorf("%s at 0 was refused by %q", k.Name, ve.Field)
			}
		}
	}
}

// A file edited by hand is refused the same way a bad value typed at the
// command line is. Before the registry these were two switches with the
// same words written out twice.
func TestAStoredChoiceIsCheckedAgainstTheRegistry(t *testing.T) {
	for _, k := range Keys() {
		if k.Kind != KindChoice {
			continue
		}
		c := Default()
		*k.str(&c) = "nonsense"
		err := c.Validate()
		ve, isValidation := err.(ValidationError)
		if !isValidation {
			t.Errorf("%s: a stored value of nonsense gave %v", k.Name, err)
			continue
		}
		if ve.Field != k.Name {
			t.Errorf("%s: refused by %q instead", k.Name, ve.Field)
			continue
		}
		// The same value typed at the command line says the same thing.
		g := Default()
		typed := k.Set(&g, "nonsense").(ValidationError)
		if typed.Detail != ve.Detail || typed.Fix != ve.Fix {
			t.Errorf("%s: stored says %q/%q, typed says %q/%q",
				k.Name, ve.Detail, ve.Fix, typed.Detail, typed.Fix)
		}
	}
}
