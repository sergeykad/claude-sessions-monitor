//go:build !linux && !darwin

package session

import (
	"fmt"
	"runtime"
)

// GetOAuthToken has no implementation on this platform.
//
// It says so rather than reporting no token, which would send the user looking
// for a sign-in that would not help.
func GetOAuthToken() (*OAuthToken, error) {
	return nil, fmt.Errorf("reading Claude Code credentials is not supported on %s", runtime.GOOS)
}
