package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestCatalogMatchesSchema validates the shipped data catalog against
// the contractual schemas/v1/catalog.schema.json — the document
// consumers (quickgetc, tooling) pin their parsers to.
func TestCatalogMatchesSchema(t *testing.T) {
	read := func(p string) any {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: not JSON: %v", p, err)
		}
		return v
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("catalog.schema.json",
		read("../../schemas/v1/catalog.schema.json")); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("catalog.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(read("../../catalog/data/catalog.json")); err != nil {
		t.Fatalf("catalog/data/catalog.json violates catalog.schema.json: %v", err)
	}
}
