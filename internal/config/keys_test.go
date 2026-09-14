package config

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// leaves reads every guard setting out of the type itself, by the JSON
// path a person would type, so the tests below compare the registry
// against the shape rather than against a list someone maintains.
func leaves(g Guard) map[string]string {
	out := map[string]string{}
	var walk func(v reflect.Value, prefix string)
	walk = func(v reflect.Value, prefix string) {
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			path := prefix + "." + name
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Struct:
				walk(f, path)
			case reflect.Int:
				out[path] = strconv.FormatInt(f.Int(), 10)
			case reflect.String:
				out[path] = f.String()
			default:
				out[path] = "unsupported kind " + f.Kind().String()
			}
		}
	}
	walk(reflect.ValueOf(g), "guard")
	return out
}

// The registry has to cover the type in both directions. A setting added
// to Guard with no key would be unreachable by name, and a key naming a
// field that is gone would offer something that cannot be set.
func TestEveryGuardSettingHasAKey(t *testing.T) {
	inType := leaves(Default().Guard)
	inRegistry := map[string]bool{}
	for _, name := range Names() {
		inRegistry[name] = true
	}
	for path := range inType {
		if !inRegistry[path] {
			t.Errorf("%s is a guard setting with no key, so nobody can change it by name", path)
		}
	}
	for name := range inRegistry {
		if _, ok := inType[name]; !ok {
			t.Errorf("the registry offers %s, which is not a field of Guard", name)
		}
	}
}

// A key holds an accessor, and an accessor can point at the wrong field
// without failing to compile. Writing through each key must change the
// one setting it names and nothing else.
func TestEachKeyWritesTheFieldItNames(t *testing.T) {
	for _, k := range Keys() {
		g := Default().Guard
		before := leaves(g)

		var next string
		if k.Kind == KindChoice {
			for _, c := range k.Choices {
				if c != before[k.Name] {
					next = c
					break
				}
			}
		} else {
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
		g := Default().Guard
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
		if k.Value(g) != k.Value(Default().Guard) {
			t.Errorf("%s: a refused value was written anyway", name)
		}
	}
}

// A choice is accepted whatever case it is typed in, which is what the
// command did before the registry.
func TestAChoiceIsReadWithoutRegardToCase(t *testing.T) {
	k, _ := Lookup("guard.mode")
	g := Default().Guard
	if err := k.Set(&g, "STRICT"); err != nil {
		t.Fatal(err)
	}
	if g.Mode != ModeStrict {
		t.Fatalf("mode %q", g.Mode)
	}
}

// A bare number in the table says nothing about what it counts.
func TestDisplayCarriesTheUnit(t *testing.T) {
	g := Default().Guard
	want := map[string]string{
		"guard.context.warn":                strconv.Itoa(g.Context.Warn) + "%",
		"guard.session.durationWarnMinutes": strconv.Itoa(g.Session.DurationWarnMinutes) + " minutes",
		"guard.session.activeSubagentsWarn": strconv.Itoa(g.Session.ActiveSubagentsWarn),
		"guard.mode":                        g.Mode,
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
		if err := k.Set(&c.Guard, "0"); err != nil {
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
