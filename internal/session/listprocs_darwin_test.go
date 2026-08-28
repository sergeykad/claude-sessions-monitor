//go:build darwin

package session

import (
	"testing"

	"golang.org/x/sys/unix"
)

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

// The orphan test is ppid == 1. Read from Proc.P_oppid, which the kernel fills
// in only under a debugger and leaves at zero otherwise, it never fires, and no
// macOS session is ever badged a ghost.
func TestKinfoToProcInfoReadsTheRealParentPID(t *testing.T) {
	var kp unix.KinfoProc
	kp.Proc.P_pid = 1868
	kp.Eproc.Ppid = 1
	kp.Proc.P_oppid = 4321
	copy(kp.Proc.P_comm[:], "claude")

	got := kinfoToProcInfo(&kp)
	want := procInfo{pid: 1868, ppid: 1, comm: "claude"}
	if got != want {
		t.Errorf("kinfoToProcInfo = %+v, want %+v", got, want)
	}
}
