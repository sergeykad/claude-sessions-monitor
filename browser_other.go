//go:build !linux && !darwin

package main

import (
	"fmt"
	"runtime"
)

// openBrowser has no implementation on this platform. It says so, so the key
// press reports why nothing happened instead of looking ignored.
func openBrowser(string) error {
	return fmt.Errorf("opening a browser is not supported on %s", runtime.GOOS)
}
