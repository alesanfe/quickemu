//go:build poc

// POC: gofrs/flock as the registry lock instead of mkdir. Behind -tags
// poc — the mkdir lock stays the default until parity is proven.
package main

import "github.com/gofrs/flock"

func lockFlock(reg string) func() {
	fl := flock.New(reg + ".flk")
	if err := fl.Lock(); err != nil {
		die("LOCK_BUSY", err.Error())
	}
	return func() { fl.Unlock() }
}
