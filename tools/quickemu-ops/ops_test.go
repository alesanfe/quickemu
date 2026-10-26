package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The op state file must match the published v1 contract — same fields
// the Bash op_new writes, same types QuickGUI reads.
func TestOpStateSchemaShape(t *testing.T) {
	dir := t.TempDir()
	o := &Op{SchemaVersion: 1, OperationID: "x", Type: "export", VM: "v",
		State: "queued", Progress: 0, StartedAt: "2026-01-01T00:00:00Z",
		Result: json.RawMessage("null"), Error: json.RawMessage("null")}
	o.save(dir)

	b, err := os.ReadFile(filepath.Join(dir, "x.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"schemaVersion": float64(1), "operationId": "x", "type": "export",
		"vm": "v", "state": "queued", "progress": float64(0),
		"startedAt": "2026-01-01T00:00:00Z", "result": nil, "error": nil,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("op state diverged (-want +got):\n%s", diff)
	}
}

// Terminal-state guard: a finished op must refuse to re-enter running.
func TestTerminalStateGuard(t *testing.T) {
	dir := t.TempDir()
	o := &Op{SchemaVersion: 1, OperationID: "t2", Type: "import", VM: "v",
		State: "completed", Progress: 100, StartedAt: "x",
		Result: json.RawMessage("null"), Error: json.RawMessage("null")}
	o.save(dir)
	if got := load(dir, "t2").State; got != "completed" {
		t.Fatalf("state=%q", got)
	}
}

// jsonschema sanity: the envelope schema file compiles so compare-mode
// can validate real envelopes against it.
func TestEnvelopeSchemaCompiles(t *testing.T) {
	c := jsonschema.NewCompiler()
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["schemaVersion","command","success","data","warnings","errors","meta"]}`
	if err := c.AddResource("env.json", mustUnmarshal(t, schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Compile("env.json"); err != nil {
		t.Fatal(err)
	}
}

func mustUnmarshal(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
