// tools/quickget-vm/cloudinit.go: first-class cloud-init seed options —
// flag parsing, base user-data generation, user-data merge (user wins,
// ssh_authorized_keys union), minimal YAML validation, secret redaction
// and the seed subcommand (--regen-seed diff before overwrite).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// seedUser is one cloud-init users[] entry: a login name plus the SSH
// authorized keys attached to it (--user/--ssh-key pairs).
type seedUser struct {
	name string
	keys []string
}

// seedOptions collects every cloud-init customisation flag shared by
// the get/mkconf/seed commands. Keys/passphrases are held here but are
// NEVER printed — every log line uses redactSeedYAML first.
type seedOptions struct {
	userDataFile string
	metaDataFile string
	networkFile  string
	packages     []string
	runcmds      []string
	users        []seedUser // index 0 is the primary login user
	explicitKeys bool       // any --ssh-key seen: disables ~/.ssh auto-detect
	hostname     string
	timezone     string
	locale       string
	regen        bool
}

var seedOpts seedOptions

// seedFlags lists the flag forms parseSeedFlag accepts (for usage/docs
// and the arg loops in main.go). Keys are unexported: they must never
// echo back their values on error.
var seedFlagNames = map[string]bool{
	"--user-data": true, "--meta-data": true, "--network-config": true,
	"--packages": true, "--runcmd": true, "--user": true,
	"--ssh-key": true, "--hostname": true, "--timezone": true,
	"--locale": true,
}

// parseSeedFlag consumes args[i] when it is a seed flag, taking the
// following arg as its value. Returns consumed=false for unrelated
// flags so the caller's own switch can handle them. --regen-seed is
// valueless; every other seed flag needs a value.
func (o *seedOptions) parseSeedFlag(args []string, i *int) (bool, error) {
	a := args[*i]
	if a == "--regen-seed" {
		o.regen = true
		return true, nil
	}
	if !seedFlagNames[a] {
		return false, nil
	}
	if *i+1 >= len(args) {
		return true, fmt.Errorf("%s requires a value", a)
	}
	v := args[*i+1]
	*i++
	switch a {
	case "--user-data":
		o.userDataFile = v
	case "--meta-data":
		o.metaDataFile = v
	case "--network-config":
		o.networkFile = v
	case "--packages":
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				o.packages = append(o.packages, p)
			}
		}
	case "--runcmd":
		o.runcmds = append(o.runcmds, v)
	case "--user":
		o.users = append(o.users, seedUser{name: safeUsername(v)})
	case "--ssh-key":
		o.explicitKeys = true
		keys, err := seedKeysFrom(v)
		if err != nil {
			return true, err
		}
		if len(o.users) == 0 {
			// A key before any --user attaches to the primary user.
			o.users = append(o.users, seedUser{})
		}
		o.users[len(o.users)-1].keys = append(o.users[len(o.users)-1].keys, keys...)
	case "--hostname":
		o.hostname = safeHostname(v)
	case "--timezone":
		o.timezone = v
	case "--locale":
		o.locale = v
	}
	return true, nil
}

// seedKeysFrom resolves one --ssh-key argument: a readable file expands
// to its key lines (one authorized-key per line, # comments skipped);
// anything else is literal key material. Key bytes are returned for the
// seed file only — errors name the file, never its contents.
func seedKeysFrom(v string) ([]string, error) {
	if st, err := os.Stat(v); err == nil && !st.IsDir() {
		b, err := os.ReadFile(v)
		if err != nil {
			return nil, fmt.Errorf("--ssh-key: %v", err)
		}
		var keys []string
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			keys = append(keys, l)
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("--ssh-key: %s has no keys", v)
		}
		return keys, nil
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("--ssh-key: empty value")
	}
	return []string{v}, nil
}

// buildUsers resolves the final users[] list: explicit --user entries in
// order (a leading key-only entry adopts the $USER name), the $USER
// default when no --user was given, and ~/.ssh auto-detection for the
// primary user when no --ssh-key was passed at all.
func (o *seedOptions) buildUsers() []seedUser {
	users := make([]seedUser, len(o.users))
	copy(users, o.users)
	if len(users) == 0 {
		users = []seedUser{{}}
	}
	if users[0].name == "" {
		users[0].name = safeUsername(os.Getenv("USER"))
	}
	if !o.explicitKeys && len(users[0].keys) == 0 {
		for _, k := range []string{"id_ed25519.pub", "id_rsa.pub"} {
			if b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", k)); err == nil {
				if k := strings.TrimSpace(string(b)); k != "" {
					users[0].keys = append(users[0].keys, k)
					break
				}
			}
		}
	}
	return users
}

// yamlScalar quotes a value for one-line YAML emission when it could
// re-parse as a non-string type or break the line (cloud-init examples
// do the same); plain scalars pass through unquoted.
func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	lower := strings.ToLower(s)
	switch lower {
	case "y", "n", "yes", "no", "true", "false", "on", "off", "null", "~",
		"none", "nan", ".nan", "inf", ".inf", "-inf", "-.inf":
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	// Leading characters that would make YAML read this as a
	// collection/comment/anchor, embedded separators that close a
	// flow scalar, or a numeric-looking literal need quoting.
	if strings.ContainsAny(s, "#'\"\\{}[],:&*?|<>@`!%") ||
		strings.Contains(s, ": ") || strings.HasSuffix(s, ":") ||
		s != strings.TrimSpace(s) || looksYAMLNum(s) {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range s {
			switch r {
			case '"', '\\':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('"')
		return b.String()
	}
	return s
}

func looksYAMLNum(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case strings.ContainsRune(".+-_ex", r):
		default:
			return false
		}
	}
	return true
}

// baseUserData renders the generated cloud-config template. It mirrors
// the gen_seed_iso block: a sudo-able primary user with the quickemu
// password, optional extra users, SSH authorized keys, and the new
// hostname/timezone/locale/packages/runcmd top-level keys.
func (o *seedOptions) baseUserData(hostName string) string {
	if o.hostname != "" {
		hostName = o.hostname
	}
	var ud strings.Builder
	ud.WriteString("#cloud-config\nhostname: " + hostName + "\nmanage_etc_hosts: true\n")
	if o.timezone != "" {
		ud.WriteString("timezone: " + yamlScalar(o.timezone) + "\n")
	}
	if o.locale != "" {
		ud.WriteString("locale: " + yamlScalar(o.locale) + "\n")
	}
	ud.WriteString("users:\n")
	for _, u := range o.buildUsers() {
		ud.WriteString("  - name: " + u.name +
			"\n    sudo: ALL=(ALL) NOPASSWD:ALL\n    groups: sudo\n    shell: /bin/bash" +
			"\n    lock_passwd: false\n    plain_text_passwd: quickemu\n")
		if len(u.keys) > 0 {
			ud.WriteString("    ssh_authorized_keys:\n")
			for _, k := range u.keys {
				ud.WriteString("      - " + yamlScalar(k) + "\n")
			}
		}
	}
	ud.WriteString("ssh_pwauth: true\npackage_update: true\n")
	if len(o.packages) > 0 {
		ud.WriteString("packages:\n")
		for _, p := range o.packages {
			ud.WriteString("  - " + yamlScalar(p) + "\n")
		}
	}
	if len(o.runcmds) > 0 {
		ud.WriteString("runcmd:\n")
		for _, c := range o.runcmds {
			ud.WriteString("  - " + yamlScalar(c) + "\n")
		}
	}
	return ud.String()
}

// parseCloudDoc is the minimal YAML validation: a document that parses
// and whose top level is a mapping. Empty/whitespace documents and
// non-mapping tops (a bare list, a scalar) are rejected.
func parseCloudDoc(doc, what string) (map[string]any, error) {
	var v any
	if err := yaml.Unmarshal([]byte(doc), &v); err != nil {
		return nil, fmt.Errorf("%s: not valid YAML: %v", what, err)
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, fmt.Errorf("%s: top level must be a YAML mapping", what)
	}
	return m, nil
}

// toStringList normalises a YAML list node to a string slice for the
// union merge; non-list values pass through unchanged (user wins as a
// plain override).
func toStringList(v any) []string {
	l, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(l))
	for _, e := range l {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// unionKeys joins two ssh_authorized_keys lists, keeping base order and
// dropping duplicates — a key the base template and the user-data both
// carry must not appear twice.
func unionKeys(base, overlay any) any {
	bl, ol := toStringList(base), toStringList(overlay)
	if bl == nil || ol == nil {
		return overlay
	}
	seen := map[string]bool{}
	out := make([]any, 0, len(bl)+len(ol))
	for _, k := range append(append([]string{}, bl...), ol...) {
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// mergeUsers merges two users[] lists by name: an overlay entry whose
// name matches a base entry wins field-for-field (except
// ssh_authorized_keys, which unions); unmatched overlay users append.
func mergeUsers(base, overlay any) any {
	bl, okB := base.([]any)
	ol, okO := overlay.([]any)
	if !okB || !okO {
		return overlay
	}
	pos := map[string]int{}
	for i, e := range bl {
		if m, ok := e.(map[string]any); ok {
			if n, ok := m["name"].(string); ok {
				pos[n] = i
			}
		}
	}
	for _, e := range ol {
		om, ok := e.(map[string]any)
		if !ok {
			continue
		}
		n, _ := om["name"].(string)
		i, found := pos[n]
		if !found {
			pos[n] = len(bl)
			bl = append(bl, e)
			continue
		}
		bm, ok := bl[i].(map[string]any)
		if !ok {
			bl[i] = e
			continue
		}
		for k, v := range om {
			if k == "ssh_authorized_keys" {
				bm[k] = unionKeys(bm[k], v)
				continue
			}
			bm[k] = v
		}
	}
	return bl
}

// Merge contract (documented for users): the generated base template is
// overlaid with the user's user-data file, top-level key by top-level
// key. The user wins on any scalar/map/list key — the two exceptions
// that MERGE rather than replace are:
//   - users: entries merge by `name` (per-field user wins), and inside a
//     merged user ssh_authorized_keys unions
//   - ssh_authorized_keys at top level: unions
//
// Everything else (packages, runcmd, hostname, timezone, locale,
// write_files, network…) REPLACES the generated value — never merged.
func mergeCloudConfig(base, overlay string) (string, error) {
	b, err := parseCloudDoc(base, "generated user-data")
	if err != nil {
		return "", err
	}
	u, err := parseCloudDoc(overlay, "user-data")
	if err != nil {
		return "", err
	}
	for k, v := range u {
		switch k {
		case "users":
			b[k] = mergeUsers(b[k], v)
		case "ssh_authorized_keys":
			b[k] = unionKeys(b[k], v)
		default:
			b[k] = v
		}
	}
	out, err := yaml.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("user-data: could not re-serialize: %v", err)
	}
	return "#cloud-config\n" + string(out), nil
}

// userDataWithOverlay builds the final user-data: the generated base
// plus the --user-data overlay merge (validation runs on the overlay
// BEFORE it can influence the seed).
func (o *seedOptions) userDataWithOverlay(hostName string) (string, error) {
	base := o.baseUserData(hostName)
	if o.userDataFile == "" {
		return base, nil
	}
	b, err := os.ReadFile(o.userDataFile)
	if err != nil {
		return "", fmt.Errorf("--user-data: %v", err)
	}
	return mergeCloudConfig(base, string(b))
}

// metaDataWithOverlay returns the --meta-data file's content validated
// as a YAML mapping; "" when the flag was not given.
func (o *seedOptions) metaDataWithOverlay() (string, error) {
	if o.metaDataFile == "" {
		return "", nil
	}
	b, err := os.ReadFile(o.metaDataFile)
	if err != nil {
		return "", fmt.Errorf("--meta-data: %v", err)
	}
	if _, err := parseCloudDoc(string(b), "meta-data"); err != nil {
		return "", err
	}
	return string(b), nil
}

// secretKey matches YAML keys whose values must never reach the log —
// SSH material and every password-ish field cloud-init accepts.
var secretKey = map[string]bool{
	"ssh_authorized_keys": true, "ssh_import_id": true,
	"passwd": true, "password": true, "plain_text_passwd": true,
	"hashed_passwd": true, "chpasswd": true, "ssh_passwd": true,
}

// redactSeedYAML blanks the sensitive VALUES in a YAML document for
// logging: scalar values of password keys, and every list item under
// key-material lists. Key names stay (the diff needs them); the bytes
// never do. The list-item pass only covers lines indented deeper than
// the secret key itself — a same/lower-indent line ends the list.
func redactSeedYAML(doc string) string {
	lines := strings.Split(doc, "\n")
	listIndent := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		indent := len(l) - len(strings.TrimLeft(l, " "))
		if listIndent >= 0 {
			if t == "" {
				continue
			}
			if indent > listIndent {
				// Anything nested under a secret key — list items,
				// block-scalar lines (chpasswd.list: |) — redacts.
				if strings.HasPrefix(t, "-") {
					lines[i] = l[:indent] + "- <redacted>"
				} else {
					lines[i] = l[:indent] + "<redacted>"
				}
				continue
			}
			listIndent = -1 // dedent ends the secret block
		}
		k := t
		if j := strings.Index(k, ":"); j >= 0 {
			k = strings.TrimSpace(k[:j])
		}
		if !secretKey[k] {
			continue
		}
		if strings.HasSuffix(t, ":") {
			listIndent = indent // redact following deeper "- item" lines
		} else if j := strings.Index(l, ":"); j >= 0 {
			lines[i] = l[:j+1] + " <redacted>"
		}
	}
	return strings.Join(lines, "\n")
}

// diffSeedDocs produces a minimal redacted unified-ish diff between an
// old and a new YAML document — the --regen-seed "what changes" report.
// Unchanged context lines collapse to a marker; secrets stay redacted.
func diffSeedDocs(oldDoc, newDoc string) string {
	oldL := strings.Split(strings.TrimRight(oldDoc, "\n"), "\n")
	newL := strings.Split(strings.TrimRight(newDoc, "\n"), "\n")
	set := map[string]int{}
	for _, l := range oldL {
		set[l]++
	}
	var out []string
	for _, l := range newL {
		if set[l] > 0 {
			set[l]--
			continue
		}
		out = append(out, "+"+l)
	}
	set = map[string]int{}
	for _, l := range newL {
		set[l]++
	}
	var removed []string
	for _, l := range oldL {
		if set[l] > 0 {
			set[l]--
			continue
		}
		removed = append(removed, "-"+l)
	}
	if len(out) == 0 && len(removed) == 0 {
		return ""
	}
	body := strings.Join(append(removed, out...), "\n")
	return redactSeedYAML(body)
}

// seedSubcommand implements `quickget-vm seed <vmdir> [seed-flags]
// [--regen-seed]`: write user-data/meta-data (+ optional
// network-config), build seed.iso and record it in the vmdir's .conf.
// Overwriting an existing seed requires --regen-seed and prints the
// redacted diff first.
func seedSubcommand(args []string) int {
	var vmPath string
	var o seedOptions
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			if vmPath == "" {
				vmPath = a
				continue
			}
			fmt.Println("seed: unexpected argument " + a)
			return 2
		}
		ok, err := o.parseSeedFlag(args, &i)
		if err != nil {
			fmt.Println("seed: " + err.Error())
			return 2
		}
		if !ok {
			fmt.Println("seed: unsupported argument " + a)
			return 2
		}
	}
	if vmPath == "" {
		fmt.Println("usage: quickget-vm seed <vmdir> [--user-data f] [--meta-data f] [--network-config f] " +
			"[--packages a,b] [--runcmd c] [--user u --ssh-key k] [--hostname h] [--timezone tz] [--locale l] [--regen-seed]")
		return 2
	}
	if st, err := os.Stat(vmPath); err != nil || !st.IsDir() {
		fmt.Println("seed: " + vmPath + " is not a VM directory")
		return 1
	}
	confFile := vmPath + ".conf"
	if _, err := os.Stat(confFile); err != nil {
		fmt.Println("seed: " + confFile + " not found")
		return 1
	}
	userData := filepath.Join(vmPath, "user-data")
	if _, err := os.Stat(userData); err == nil && !o.regen {
		fmt.Println("seed: seed already exists for " + vmPath + " (pass --regen-seed to rebuild it)")
		return 1
	}
	return genSeedISOOpts(vmPath, confFile, &o)
}
