// quickemu-plugin-market: template marketplace client.
// Fetches a JSON catalog of conf templates (MARKET_CATALOG overrides
// the default URL — http(s) or local file path) and lists, shows or
// installs them as ready-to-run .conf files. Replaces the former
// curl+jq bash script.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const defaultCatalog = "https://raw.githubusercontent.com/alesanfe/quickemu/master/catalog/market.json"

type template struct {
	Name string `json:"name"`
	Conf string `json:"conf"`
}
type catalog struct {
	Templates []template `json:"templates"`
}

func load() catalog {
	src := os.Getenv("MARKET_CATALOG")
	if src == "" {
		src = defaultCatalog
	}
	var b []byte
	if u, err := url.Parse(src); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		s, err := qelib.FetchString(src, 4<<20)
		if err != nil {
			fmt.Fprintf(os.Stderr, "market: fetch: %v\n", err)
			os.Exit(1)
		}
		b = []byte(s)
	} else {
		var err error
		if b, err = os.ReadFile(src); err != nil {
			fmt.Fprintf(os.Stderr, "market: read %s: %v\n", src, err)
			os.Exit(1)
		}
	}
	var c catalog
	if err := json.Unmarshal(b, &c); err != nil {
		fmt.Fprintf(os.Stderr, "market: bad catalog: %v\n", err)
		os.Exit(1)
	}
	return c
}

func find(c catalog, name string) *template {
	for i := range c.Templates {
		if c.Templates[i].Name == name {
			return &c.Templates[i]
		}
	}
	return nil
}

// writeConf normalizes the template body: exactly one trailing
// newline so the installed .conf passes POSIX text-file checks.
func writeConf(path, conf string) error {
	return os.WriteFile(path, []byte(strings.TrimRight(conf, "\n")+"\n"), 0o644)
}

func main() { os.Exit(run()) }

func run() int {
	args := os.Args[1:]
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-plugin-market {list|show NAME|install NAME [DIR]}")
		return 2
	}
	c := load()
	switch args[0] {
	case "list":
		for _, tp := range c.Templates {
			fmt.Println(tp.Name)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: show NAME")
			return 2
		}
		tp := find(c, args[1])
		if tp == nil {
			fmt.Fprintf(os.Stderr, "market: %s not found\n", args[1])
			return 1
		}
		b, _ := json.Marshal(tp)
		fmt.Println(string(b))
	case "install":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: install NAME [DIR]")
			return 2
		}
		dir := "."
		if len(args) > 2 {
			dir = args[2]
		}
		tp := find(c, args[1])
		if tp == nil {
			fmt.Fprintf(os.Stderr, "market: %s not found\n", args[1])
			return 1
		}
		out := filepath.Join(dir, args[1]+".conf")
		if err := writeConf(out, tp.Conf); err != nil {
			fmt.Fprintf(os.Stderr, "market: %v\n", err)
			return 1
		}
		fmt.Printf("installed %s\n", out)
	default:
		fmt.Fprintln(os.Stderr, "usage: quickemu-plugin-market {list|show NAME|install NAME [DIR]}")
		return 2
	}
	return 0
}
