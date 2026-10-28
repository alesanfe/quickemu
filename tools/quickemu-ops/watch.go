// ops watch: stream operation-state changes as JSONL so QuickGUI stops
// polling --op-list. fsnotify with a polling reconciliation fallback.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// runWatch streams op-file changes to out — parameterized so tests do
// not race on the os.Stdout global with the still-running watcher
// goroutine (the watch loop runs until killed by design).
func runWatch(dir string, out io.Writer) int {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		// Fallback: poll mtimes every 500ms when notify isn't available
		// (NFS/FUSE filesystems don't reliably emit events).
		seen := map[string]time.Time{}
		for {
			ents, _ := filepath.Glob(filepath.Join(dir, "*.json"))
			for _, e := range ents {
				if st, err := os.Stat(e); err == nil {
					if t, ok := seen[e]; !ok || st.ModTime().After(t) {
						seen[e] = st.ModTime()
						b, _ := os.ReadFile(e)
						fmt.Fprint(out, string(b))
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		die("WATCH_UNAVAILABLE", err.Error())
	}
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return 0
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 &&
				filepath.Ext(ev.Name) == ".json" {
				if b, err := os.ReadFile(ev.Name); err == nil {
					fmt.Fprint(out, string(b))
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return 0
			}
			fmt.Fprintln(os.Stderr, "watch error:", err)
		}
	}
}
