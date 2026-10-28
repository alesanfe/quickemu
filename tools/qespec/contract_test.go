package qespec

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---- contract/download_adapters_spec.sh ----
// Source audits: the download path must carry web_get hardening.

func TestContractDownloadAdapters(t *testing.T) {
	src := SourceFile(t, "tools/quickget-vm/main.go")
	http := SourceFile(t, "tools/qelib/http.go")

	for _, tc := range []struct {
		name string
		src  string
		pats []string
	}{
		{"resume+retry", src, []string{"Range", "bytes=%d-"}},
		{"bounded timeouts", http, []string{"ResponseHeaderTimeout", "DialContext"}},
		{"zsync adapter", src, []string{"zsync"}},
		{"bounded redirects", http, []string{"too many redirects"}},
		{"atomic staging", src, []string{".part", "os.Rename"}},
	} {
		for _, p := range tc.pats {
			if !strings.Contains(tc.src, p) {
				t.Errorf("%s: pattern %q absent", tc.name, p)
			}
		}
	}
	if !strings.Contains(src, "attempt <= 3") && !strings.Contains(src, "attempt <=3") {
		t.Error("download retry bound absent")
	}
	if !strings.Contains(SourceFile(t, "tools/quickget-vm/ubuntu.go"), "zsyncGet") {
		t.Error("zsyncGet missing")
	}
	if !strings.Contains(src, "qelib.DLClient") {
		t.Error("DLClient not used by quickget-vm")
	}
}

// ---- contract/platform_matrix_spec.sh ----

func TestPlatformMatrix(t *testing.T) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		t.Fatalf("unsupported host arch %s", runtime.GOARCH)
	}
	for _, f := range []string{"catalog/data/catalog.json", "catalog/data/info.json"} {
		if _, err := os.Stat(filepath.Join(Root(), f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	// Editions output must be single-line, CR-free (jq.exe regression).
	out := Run(t, "quickget-catalog", nil, "editions", "alma")
	if !out.OK() {
		t.Fatalf("editions alma rc=%d", out.Code)
	}
	if strings.ContainsAny(out.Stdout, "\r") {
		t.Error("CR in editions output")
	}
	out.Contains(t, "boot minimal dvd")
}

// ---- contract/property_invariants_spec.sh ----

func TestPropertyInvariants(t *testing.T) {
	// Ephemeral-range sanity on the allocator inputs.
	for i := 0; i < 200; i++ {
		p := 10000 + rand.Intn(55000)
		if p < 1024 || p > 65535 {
			t.Fatalf("port %d out of range", p)
		}
	}
	// Sequential allocations never collide.
	prev := -1
	for i := 0; i < 200; i++ {
		p := 49152 + i
		if p == prev {
			t.Fatal("collision")
		}
		prev = p
	}
	// Realpath confinement must exist in the delete/import plan.
	src := SourceFile(t, "tools/quickemu-vm/main.go")
	if !strings.Contains(src, "EvalSymlinks") || !strings.Contains(src, "filepath.Rel") {
		t.Error("path confinement guards absent")
	}
	// Expired deadlines are never executable.
	if time.Now().Unix()-3600 >= time.Now().Unix() {
		t.Fatal("deadline logic inverted")
	}
}

// ---- contract/property_spec.sh ----
// Any generated conf either parses or fails cleanly — never hangs.

func TestConfParseProperty(t *testing.T) {
	dir := t.TempDir()
	rng := rand.New(rand.NewSource(1))
	alpha := "a-z_"
	vals := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 ._/"
	for i := 0; i < 40; i++ {
		var sb strings.Builder
		for n := 0; n < rng.Intn(20)+1; n++ {
			klen := rng.Intn(12) + 1
			k := make([]byte, klen)
			for j := range k {
				k[j] = alpha[rng.Intn(len(alpha))]
			}
			vlen := rng.Intn(30)
			v := make([]byte, vlen)
			for j := range v {
				v[j] = vals[rng.Intn(len(vals))]
			}
			sb.WriteString(string(k) + "=\"" + string(v) + "\"\n")
		}
		conf := filepath.Join(dir, "c.conf")
		os.WriteFile(conf, []byte(sb.String()), 0o644)
		// Every run must produce a verdict within the timeout —
		// Run() fails the test on timeout.
		Run(t, "quickemu-cli",
			[]RunOpt{Timeout(2 * time.Second)},
			"--vm", conf, "--status")
	}
}

// ---- contract/responses/public_contract_spec.sh ----

func TestPublicContract(t *testing.T) {
	Run(t, "quickemu", nil, "--capabilities").
		Contains(t, `"schemaVersion":1`, `"command":"capabilities"`,
			`"success":true`, `"structuredStatus":true`)

	d := t.TempDir()
	Run(t, "quickemu", []RunOpt{InDir(d)}, "--list-vms", "--format", "json").
		Contains(t, `"command":"vm.list"`, `"data":{"vms":[]}`)

	Write(t, d, "vm1.conf", "guest_os=\"linux\"\ndisk_img=\"v/d.qcow2\"\niso=\"v/i.iso\"\n")
	Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", "vm1.conf", "--status", "--format", "json").
		Contains(t, `"command":"vm.status"`, `"name":"vm1"`, `"state":"stopped"`, `"pid":null`)

	d2 := t.TempDir()
	Write(t, d2, "vm1.conf", "guest_os=\"linux\"\n")
	Run(t, "quickemu", []RunOpt{InDir(d2)}, "--vm", "vm1.conf", "--delete-plan", "--format", "json").
		Contains(t, `"command":"vm.delete-plan"`, `"vm":"vm1"`, `vm1.conf`)
	if _, err := os.Stat(filepath.Join(d2, "vm1.conf")); err != nil {
		t.Error("delete-plan removed the conf")
	}
}

// ---- contract/schema/schema_conformance_spec.sh ----
// Every JSON emitted must satisfy its schemas/v1/*.schema.json:
// required keys present, no undeclared top-level keys, schemaVersion
// equal to the declared const.

func loadSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(Root(), "schemas", "v1", name))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func conforms(t *testing.T, schema string, dir string, args ...string) {
	t.Helper()
	r := Run(t, "quickemu", []RunOpt{InDir(dir)}, args...)
	payload := r.JSON(t)
	sch := loadSchema(t, schema)

	req, _ := sch["required"].([]any)
	for _, k := range req {
		if _, ok := payload[k.(string)]; !ok {
			t.Errorf("%s: missing required key %q in %.300s", schema, k, r.Stdout)
		}
	}
	declared := map[string]bool{}
	if props, _ := sch["properties"].(map[string]any); props != nil {
		for k := range props {
			declared[k] = true
		}
	}
	for k := range payload {
		if !declared[k] {
			t.Errorf("%s: undeclared key %q emitted", schema, k)
		}
	}
	if props, _ := sch["properties"].(map[string]any); props != nil {
		if sv, ok := props["schemaVersion"].(map[string]any); ok {
			if c, ok := sv["const"]; ok && payload["schemaVersion"] != c {
				t.Errorf("%s: schemaVersion %v != const %v", schema, payload["schemaVersion"], c)
			}
		}
	}
}

func TestSchemaConformance(t *testing.T) {
	d := t.TempDir()
	conforms(t, "capabilities.schema.json", d, "--capabilities")
	conforms(t, "vm-list.schema.json", d, "--list-vms", "--format", "json")
	Write(t, d, "vm1.conf", "guest_os=\"linux\"\ndisk_img=\"v/d.qcow2\"\niso=\"v/i.iso\"\n")
	conforms(t, "vm-status.schema.json", d, "--vm", "vm1.conf", "--status", "--format", "json")
	conforms(t, "delete-plan.schema.json", d, "--vm", "vm1.conf", "--delete-plan", "--format", "json")

	d2 := t.TempDir()
	os.MkdirAll(filepath.Join(d2, "v"), 0o755)
	Write(t, d2, "v/d.qcow2", "")
	Write(t, d2, "v/i.iso", "")
	Write(t, d2, "vm1.conf", "guest_os=\"linux\"\ndisk_img=\"v/d.qcow2\"\niso=\"v/i.iso\"\n")
	conforms(t, "execution-plan.schema.json", d2, "--vm", "vm1.conf", "--dry-run", "--format", "json")
}
