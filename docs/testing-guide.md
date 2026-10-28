# Testing guide — seams, isolation, and subprocess fakes

Rules for writing tests against the Go backend. The short version:
stubs are package-level function variables (ADR-0009); they are
process-global, so mutate them only from non-parallel tests and always
restore them.

## Seam inventory

Every package that forks subprocesses exposes one construction seam:

```go
var execCommand = exec.Command   // tools/*/seam.go
```

Plus dependency seams where they exist:

| Package | Seam | Fakes |
|---|---|---|
| all tools | `execCommand` | `exec.Command` construction |
| dev | `execCommand` | same pattern (predates seam.go) |
| quickget-vm | `fetch`, `fetchString`, `httpProbe` | HTTP GET / probe client |
| quickget-cli | `fetch`, `fetchString` | catalog HTTP |
| quickemu-api | `newMux` (handler tree) | in-process httptest |

## Rules

1. **Restore with `t.Cleanup`** — never `defer` alone at function top
   if the helper installs more than one stub:

   ```go
   old := execCommand
   execCommand = fake
   t.Cleanup(func() { execCommand = old })
   ```

2. **Never `t.Parallel()` while a global seam is stubbed.** Tests in
   the same package share the process. If a test needs parallelism,
   take the dependency as a parameter instead of a global.

3. **Ordered matchers, not maps.** Map iteration order is random —
   a URL matching two stub keys picks one at random (this produced a
   flaky -race run). Use a slice of `(match, response)` pairs checked
   in order, most-specific first.

4. **Prefer dependency parameters over globals for new code.**
   `runWatch(dir, out io.Writer)` beats reassigning `os.Stdout` — a
   leaked goroutine racing on the global broke `-race` before.

5. **Fake `*exec.Cmd` construction, not execution.** Stub
   `execCommand` to return a `*exec.Cmd` wrapping a test binary or a
   `exec.Command("sh", "-c", ...)` helper; the production code path
   (`cmd.Run`, `CombinedOutput`, `Wait`) still executes for real.

6. **Cleanup leaks:** watchers/goroutines started in tests must be
   cancellable or bounded; a `for{}` loop goroutine that outlives the
   test still reads captured globals (`-race` flags it).

## exec.Command classification

All direct `exec.Command` sites now route through `execCommand`
(seam.go per package). Classification, kept for context:

- **Mutating** — qemu-img create/convert/commit/resize, systemctl,
  sudo modprobe/initramfs, isoinfo/7z extraction, zsync, snapshot
  ops: seam mandatory (done).
- **Probes** — `qemu-system-* --version`, `-audio-help`,
  `qemu-img info`: read-only, seamed for consistency.
- **Launchers** — virt-viewer/spicy/browser openers: fire-and-forget,
  seamed so tests can assert invocation without opening windows.
- **qespec harness** — `exec.CommandContext` in helpers.go stays
  direct: it *is* the test boundary, wrapping real binaries with a
  deadline; faking it would fake the thing under test.

## Running

```sh
cd tools/<module> && go test ./...
go test -race ./...          # from module dir
task quality                 # full gate: fmt+vet+test+race+qespec
```
