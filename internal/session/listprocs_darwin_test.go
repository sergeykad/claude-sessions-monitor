//go:build darwin

package session

import "testing"

// P_comm is a fixed-width array, so the name comes back padded with NUL bytes.
// Left in, those trailing bytes make the suffix match fail for every process
// and csm reports no running sessions on macOS.
func TestCommStringDropsTheNULPadding(t *testing.T) {
	padded := make([]byte, 17)
	copy(padded, "claude")

	if got := commString(padded); got != "claude" {
		t.Errorf("commString = %q, want %q", got, "claude")
	}
	// A name that fills the array leaves no terminator to stop at.
	full := []byte("sixteencharsname")
	if got := commString(full); got != "sixteencharsname" {
		t.Errorf("commString on an unterminated name = %q", got)
	}
}
