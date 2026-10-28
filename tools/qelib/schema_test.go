package qelib

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// captureStdout grabs one synchronous Emit call's output — Emit
// returns before the writer is restored, so no goroutine races on
// the swapped global (unlike the leaked-watcher case).
func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

// TestEmitMatchesResponseSchema validates the produced envelope
// against the contractual schemas/v1/response.schema.json — the
// same document QuickGUI consumes.
func TestEmitMatchesResponseSchema(t *testing.T) {
	schemaBytes, err := os.ReadFile("../../schemas/v1/response.schema.json")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	var doc any
	if err := json.Unmarshal(schemaBytes, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("response.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("response.schema.json")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		emit func()
	}{
		{"success", func() { EmitOK("vm-list", "test/1.0", map[string]any{"vms": []any{}}) }},
		{"success-nil-data", func() { Emit("status", "test/1.0", true, nil, nil) }},
		{"error-typed", func() {
			Emit("stop", "test/1.0", false, nil, []QEError{
				{Code: "NOT_RUNNING", Message: "vm is not running", Severity: "error", Retryable: false}})
		}},
		{"error-details", func() {
			Emit("import", "test/1.0", false, nil, []any{map[string]any{
				"code": "BAD_ARCHIVE", "message": "x", "severity": "error",
				"retryable": true, "details": map[string]any{"member": "../etc"}}})
		}},
	}
	for _, tc := range cases {
		out := captureStdout(tc.emit)
		var v any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("%s: not JSON: %v\n%s", tc.name, err, out)
		}
		if err := sch.Validate(v); err != nil {
			t.Errorf("%s: envelope violates response.schema.json: %v\n%s", tc.name, err, out)
		}
	}
	// operationId flows from OPID into meta.
	t.Setenv("OPID", "op-123")
	out := captureStdout(func() { EmitOK("x", "t/1", nil) })
	if !bytes.Contains([]byte(out), []byte(`"operationId":"op-123"`)) {
		t.Fatalf("operationId missing: %s", out)
	}
}
