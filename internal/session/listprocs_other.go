//go:build !linux && !darwin

package session

import (
	"fmt"
	"runtime"
)

// listProcessesNative has no implementation on this platform.
//
// It reports that rather than returning an empty list. An empty list means "no
// Claude session is running", and csm would say so with confidence on a machine
// where it cannot look at all. Releases build darwin and linux only.
func listProcessesNative() ([]procInfo, error) {
	return nil, fmt.Errorf("scanning processes is not supported on %s", runtime.GOOS)
}

// processComm has no implementation on this platform. See listProcessesNative.
func processComm(int) (string, error) {
	return "", fmt.Errorf("reading a process name is not supported on %s", runtime.GOOS)
}

// getProcessCwd has no implementation on this platform. See listProcessesNative.
func getProcessCwd(int) (string, error) {
	return "", fmt.Errorf("%w: not supported on %s", errCwdLookupBroken, runtime.GOOS)
}
