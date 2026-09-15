package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
)

// The README shows the file init writes. It is the first thing a reader
// sees of the configuration, and it had lost a whole section: the same
// document told them to change report.retentionDays in a file its own
// picture did not contain.
func TestTheReadmeShowsTheFileThatIsActuallyWritten(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	prose := string(b)

	const anchor = "The file it writes looks like this:"
	i := strings.Index(prose, anchor)
	if i < 0 {
		t.Fatalf("the README no longer says %q, so this test checks nothing", anchor)
	}
	block := regexp.MustCompile("(?s)```json\n(.*?)```").FindStringSubmatch(prose[i:])
	if block == nil {
		t.Fatal("no JSON block follows that sentence")
	}

	var shown map[string]json.RawMessage
	if err := json.Unmarshal([]byte(block[1]), &shown); err != nil {
		t.Fatalf("the block the README shows is not valid JSON: %v", err)
	}

	// The sections come from the type, so one added later has to appear
	// in the picture without anyone remembering this test.
	written := map[string]bool{}
	ct := reflect.TypeOf(config.Config{})
	for i := 0; i < ct.NumField(); i++ {
		name := strings.Split(ct.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		written[name] = true
		if _, ok := shown[name]; !ok {
			t.Errorf("init writes %q and the README does not show it", name)
		}
	}
	for name := range shown {
		if !written[name] {
			t.Errorf("the README shows %q, which init does not write", name)
		}
	}

	// A picture of the file has to be a file, or a reader who copies it
	// gets an error rather than a configuration.
	var c config.Config
	if err := json.Unmarshal([]byte(block[1]), &c); err != nil {
		t.Fatalf("the block does not read as a configuration: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("the configuration the README shows would be refused: %v", err)
	}
}
