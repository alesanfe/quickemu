package main

import "os/exec"

// Command construction seam (ADR-0009): tests substitute stubs to
// observe or fake subprocesses without forking. Restore the original
// via t.Cleanup; never use t.Parallel while a stub is installed.
var execCommand = exec.Command
