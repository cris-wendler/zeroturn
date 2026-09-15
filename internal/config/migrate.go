package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Migration carries a configuration file forward to the version this build
// supports, and reports what it had to do. A policy a developer wrote is
// worth more than the file format it is written in, so a file from an
// older build is read rather than refused.
//
// Nothing here lists the settings. What is missing is worked out by
// comparing the file with the key registry, so a setting added later is
// migrated without anyone adding a case for it.

// Report says what a migration found. Nothing in it is a failure: a file
// can be read and still be worth telling somebody about.
type Report struct {
	// From is the version the file declared. A file with no version at
	// all reports 0, which is what a hand written fragment looks like.
	From int
	// Filled names the settings the file did not carry, which now hold
	// the value a new file would have.
	Filled []string
	// Unknown names the settings in the file that this build does not
	// have. They are usually a spelling mistake, and sometimes a setting
	// a later version removed, so they are reported rather than dropped
	// in silence.
	Unknown []string
}

// Changed reports whether rewriting the file would alter it.
func (r Report) Changed() bool {
	return r.From != Version || len(r.Filled) > 0 || len(r.Unknown) > 0
}

// ErrNewer is returned for a file this build is too old to read. It is
// separate because the answer is to upgrade ZeroTurn, and every other
// version problem is answered by migrating the file.
type ErrNewer struct {
	FileVersion  int
	BuildVersion int
}

func (e ErrNewer) Error() string {
	return fmt.Sprintf("%s declares version %d, and this build of ZeroTurn supports version %d",
		FileName, e.FileVersion, e.BuildVersion)
}

// Migrate reads a configuration file of any supported version. Values the
// file does not carry take the value a new file would have, which is what
// makes a setting added in a later version readable by a file written
// before it existed.
func Migrate(raw []byte) (Config, Report, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return Config{}, Report{}, fmt.Errorf("%s is not valid JSON: %v", FileName, err)
	}

	r := Report{}
	if v, ok := top["version"]; ok {
		if err := json.Unmarshal(v, &r.From); err != nil {
			return Config{}, Report{}, ValidationError{"version", "value is not a whole number",
				"set version to " + fmt.Sprint(Version)}
		}
	}
	if r.From > Version {
		return Config{}, r, ErrNewer{FileVersion: r.From, BuildVersion: Version}
	}

	// Decoding on top of the defaults is the migration. A field the file
	// does not carry is left as it was, which is the value a new file
	// would hold, so every setting added after this file was written
	// arrives with its default rather than with a zero.
	c := Default()
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, r, fmt.Errorf("%s is not valid ZeroTurn configuration: %v", FileName, err)
	}
	c.Version = Version

	present := leafPaths(top)
	for _, k := range keys {
		if !present[k.Name] {
			r.Filled = append(r.Filled, k.Name)
		}
	}
	r.Unknown = unknownSettings(present)
	return c, r, nil
}

// leafPaths reports every value in the file by the dotted name a person
// would type, so it can be compared with the registry. An array is a leaf:
// the registry describes single values, and verify steps are not settings.
func leafPaths(top map[string]json.RawMessage) map[string]bool {
	out := map[string]bool{}
	var walk func(node map[string]json.RawMessage, prefix string)
	walk = func(node map[string]json.RawMessage, prefix string) {
		for name, raw := range node {
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			var child map[string]json.RawMessage
			if json.Unmarshal(raw, &child) == nil && child != nil {
				walk(child, path)
				continue
			}
			out[path] = true
		}
	}
	walk(top, "")
	return out
}

// unknownSettings names values in the file that the Config type has no
// place for. The names it compares against come from the type itself, so
// a field added or renamed there needs no change here.
func unknownSettings(present map[string]bool) []string {
	known := typePaths()
	var out []string
	for path := range present {
		if !known[path] {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// typePaths reports every value Config can hold, by the dotted name a
// person would type. A field that is not a struct is a leaf, which is why
// verify steps and the protected branch list stop here rather than being
// walked: they are values, not settings.
func typePaths() map[string]bool {
	out := map[string]bool{}
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type, path)
				continue
			}
			out[path] = true
		}
	}
	walk(reflect.TypeOf(Config{}), "")
	return out
}
