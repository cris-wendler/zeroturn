// Package settings reads and writes a harness settings file without
// disturbing what it did not put there.
//
// A settings file belongs to the person using it. Keys this package does
// not recognise keep their content, and every key keeps its position, so
// a rewrite shows only the change that was asked for. Values are held as
// raw JSON for the same reason: decoding into a type would silently drop
// any field that type does not declare.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cris-wendler/zeroturn/internal/state"
)

// ErrInvalidJSON reports a settings file that could not be parsed. The
// caller decides what to say about it, because it is the one that knows
// which command the reader is standing in.
var ErrInvalidJSON = errors.New("the settings file is not valid JSON")

// File is one settings file, held open in memory.
type File struct {
	// Path is where the file will be written.
	Path string
	// Exists reports whether the file was there when it was read. A file
	// that was not there is treated as an empty object.
	Exists bool
	// Top holds the top level values as raw JSON.
	Top map[string]json.RawMessage

	order []string
}

// Read loads a settings file. A file that does not exist is not an
// error: it reads as empty, because writing it is how it gets created.
func Read(path string) (*File, error) {
	raw, err := ioutil.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if !exists {
		raw = []byte("{}")
	}
	top := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, ErrInvalidJSON
	}
	return &File{Path: path, Exists: exists, Top: top, order: topLevelOrder(raw)}, nil
}

// Keys reports the top level keys in the order they appeared in the
// file. Keys added since are at the end.
func (f *File) Keys() []string {
	out := make([]string, len(f.order))
	copy(out, f.order)
	return out
}

// Set writes one top level value, at the end of the file when the key is
// new and in place when it is not.
func (f *File) Set(key string, value json.RawMessage) {
	f.Top[key] = value
	for _, k := range f.order {
		if k == key {
			return
		}
	}
	f.order = append(f.order, key)
}

// Delete removes one top level value.
func (f *File) Delete(key string) {
	delete(f.Top, key)
	var out []string
	for _, k := range f.order {
		if k != key {
			out = append(out, k)
		}
	}
	f.order = out
}

// Section decodes one top level value into a map of raw values, and
// reports ErrInvalidJSON when it holds something else. An absent key
// reads as empty, so a caller can add to a section that is not there.
func (f *File) Section(key string) (map[string][]json.RawMessage, error) {
	out := map[string][]json.RawMessage{}
	v, ok := f.Top[key]
	if !ok {
		return out, nil
	}
	if err := json.Unmarshal(v, &out); err != nil {
		return nil, ErrInvalidJSON
	}
	return out, nil
}

// Backup copies the file as it stands on disk to dst.
func (f *File) Backup(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	b, err := ioutil.ReadFile(f.Path)
	if err != nil {
		return err
	}
	return state.AtomicWrite(dst, b, 0600)
}

// Write rebuilds the file, keeping the original key order, and writes it
// atomically so an interruption cannot leave a partial settings file
// behind. A harness reads this file at startup, and half of one would
// stop it starting.
func (f *File) Write() error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
		return err
	}
	if len(f.Top) == 0 {
		// Removing the last entry leaves an empty settings file rather
		// than a file with a stray blank line in it.
		return state.AtomicWrite(f.Path, []byte("{}\n"), 0644)
	}
	seen := map[string]bool{}
	var buf bytes.Buffer
	buf.WriteString("{\n")
	first := true
	emit := func(k string) error {
		v, ok := f.Top[k]
		if !ok || seen[k] {
			return nil
		}
		seen[k] = true
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.WriteString("  ")
		buf.Write(kb)
		buf.WriteString(": ")
		var indented bytes.Buffer
		if err := json.Indent(&indented, v, "  ", "  "); err != nil {
			return err
		}
		buf.Write(indented.Bytes())
		return nil
	}
	for _, k := range f.order {
		if err := emit(k); err != nil {
			return err
		}
	}
	// A key added without going through Set still has to be written.
	rest := make([]string, 0, len(f.Top))
	for k := range f.Top {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sortStrings(rest)
	for _, k := range rest {
		if err := emit(k); err != nil {
			return err
		}
	}
	buf.WriteString("\n}\n")
	return state.AtomicWrite(f.Path, buf.Bytes(), 0644)
}

// topLevelOrder records the order of keys in the original file so that an
// unrelated setting does not move when the file is rewritten.
func topLevelOrder(raw []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	depth := 0
	for {
		t, err := dec.Token()
		if err == io.EOF || err != nil {
			return keys
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				if depth == 0 {
					return keys
				}
				depth--
			}
			continue
		}
		if depth == 0 {
			if s, ok := t.(string); ok {
				keys = append(keys, s)
				var skip json.RawMessage
				if dec.Decode(&skip) != nil {
					return keys
				}
			}
		}
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
