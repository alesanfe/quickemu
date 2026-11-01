// net.go: small network helpers — the embedded answer-file server
// and a plain HTTP download for the virtio-win driver ISO.
package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// startSeedHTTPServer serves dir over loopback HTTP and returns the
// bound port — slirp guests reach the host at 10.0.2.2, so a d-i
// `preseed/url=http://10.0.2.2:PORT/...` lands here. POST /done drops
// a .install-done marker file: installers that can call back (proxmox
// post-installation-webhook) signal completion over the same channel
// instead of a poweroff or serial scrape. The server lives until the
// run closes it at reconfigure (or exits).
func startSeedHTTPServer(dir string) (int, *http.Server, error) {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err := os.WriteFile(dir+"/.install-done", body, 0o644); err == nil {
			w.WriteHeader(http.StatusNoContent)
		} else {
			http.Error(w, "marker", http.StatusInternalServerError)
		}
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().(*net.TCPAddr).Port, srv, nil
}

// fetchFile downloads url to dest — used for the virtio-win ISO the
// Windows provider needs when quickget was told --disable-unattended.
// Writes via a .part file so a torn download can't masquerade as done.
func fetchFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	part := dest + ".part"
	w, err := os.Create(part)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, resp.Body)
	cerr := w.Close()
	if err != nil {
		os.Remove(part)
		return err
	}
	if cerr != nil {
		os.Remove(part)
		return cerr
	}
	return os.Rename(part, dest)
}
