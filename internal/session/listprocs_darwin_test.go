//go:build darwin

package session

import (
	"testing"

	"golang.org/x/sys/unix"
)

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
