// Command validate checks a JSON document against one of the published
// schemas in schemas/. An adapter author can run it on the events their
// adapter produces, before sending them to zeroturn.
//
// Usage: go run ./conformance/validate schemas/normalized-event.schema.json event.json
//
// With one argument the document is read from standard input. The exit
// code is 0 when the document follows the schema and 1 when it does not.
package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cris-wendler/zeroturn/internal/jsonschema"
)

func main() {
	args := os.Args[1:]
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: validate <schema.json> [document.json]")
		os.Exit(2)
	}
	schemaBytes, err := ioutil.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "the schema could not be read:", err)
		os.Exit(2)
	}
	schema, err := jsonschema.Parse(schemaBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	var doc []byte
	if len(args) == 2 {
		doc, err = ioutil.ReadFile(args[1])
	} else {
		doc, err = ioutil.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "the document could not be read:", err)
		os.Exit(2)
	}

	problems := schema.Validate(doc)
	if len(problems) == 0 {
		fmt.Println("the document follows", args[0])
		return
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	fmt.Fprintf(os.Stderr, "\n%d problems. Correct them, then run the check again.\n", len(problems))
	os.Exit(1)
}
