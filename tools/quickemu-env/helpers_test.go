package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// captureOut runs f with os.Stdout redirected to a pipe and returns
// everything written. Progress output is global, so nothing here may
// run under t.Parallel.
func captureOut(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	w.Close()
	os.Stdout = old
	return <-done
}

// stub replaces a package-level seam var for the test duration.
func stub[T any](t *testing.T, slot *T, v T) {
	t.Helper()
	old := *slot
	*slot = v
	t.Cleanup(func() { *slot = old })
}

// toolCall is one recorded runTool invocation.
type toolCall struct {
	name string
	dir  string
	args []string
}

// stubTools points the subprocess seams at deterministic fakes:
// runTool records each call and answers via fn, findTool reports "no
// sibling anywhere" and sleeper is a no-op so retry loops run fast.
func stubTools(t *testing.T, fn func(c toolCall) (string, error)) *[]toolCall {
	t.Helper()
	calls := &[]toolCall{}
	stub(t, &runTool, func(name, dir string, env []string, args ...string) (string, error) {
		c := toolCall{name, dir, append([]string{}, args...)}
		*calls = append(*calls, c)
		if fn == nil {
			return "", nil
		}
		return fn(c)
	})
	stub(t, &findTool, func(string) (string, error) {
		return "", fmt.Errorf("no tool in test")
	})
	stub(t, &sleeper, func(time.Duration) {})
	return calls
}

// stubToolsSibling is stubTools with a resolvable quickemu-ports
// sibling — findTool must be re-stubbed after each stubTools call
// since stubTools restores the "no sibling" default.
func stubToolsSibling(t *testing.T, fn func(c toolCall) (string, error)) *[]toolCall {
	t.Helper()
	calls := stubTools(t, fn)
	stub(t, &findTool, func(string) (string, error) {
		return "/x/quickemu-ports", nil
	})
	return calls
}

// writeEnvFile drops body into dir/env.toml and returns its path.
func writeEnvFile(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "env.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write env.toml: %v", err)
	}
	return p
}

// loadGood parses+validates an env.toml that must be clean.
func loadGood(t *testing.T, dir, body string) *Env {
	t.Helper()
	e, errs := loadEnv(writeEnvFile(t, dir, body))
	if len(errs) > 0 {
		t.Fatalf("loadEnv: %v\n--- doc ---\n%s", errs, body)
	}
	return e
}

// loadErrs parses+validates an env.toml that must produce at least
// one diagnostic; the joined error text is returned for substring
// assertions.
func loadErrs(t *testing.T, dir, body string) string {
	t.Helper()
	_, errs := loadEnv(writeEnvFile(t, dir, body))
	if len(errs) == 0 {
		t.Fatalf("want validation errors, got none\n--- doc ---\n%s", body)
	}
	return errs.Error()
}

// writeFile is a t.TempDir-friendly os.WriteFile that fails the test
// on error.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fakeScript writes an executable shell script into a temp dir and
// returns its path — used for findTool/execCommand stubs that need a
// real binary behind them.
func fakeScript(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("script %s: %v", p, err)
	}
	return p
}

// withStdin swaps os.Stdin for a pipe containing input for the test
// duration (destroy confirmation prompt).
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("stdin write: %v", err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		r.Close()
	})
}

// listenTCP opens a 127.0.0.1 listener for health-probe tests and
// returns its port. Each accepted connection is handled by serve
// (nil = accept and hold).
func listenTCP(t *testing.T, serve func(net.Conn)) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			if serve == nil {
				c.Close()
				continue
			}
			go serve(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// confAt writes a minimal quickemu .conf and returns its path.
func confAt(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	writeFile(t, p, body)
	return p
}
