//go:build darwin

package session

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"
)

// listProcessesNative reads the process table out of the kernel with
// sysctl kern.proc.all.
//
// An error is reported rather than an empty list. An empty list and a broken
// scan look identical downstream: every session would be marked inactive and
// csm would report no running sessions.
func listProcessesNative() ([]procInfo, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("sysctl kern.proc.all: %w", err)
	}

	// SysctlKinfoProcSlice reports an empty list, not an error, when the size
	// probe comes back zero. No machine has zero processes, and an empty list
	// downstream means "no Claude session is running", which csm would then
	// state with confidence.
	if len(procs) == 0 {
		return nil, errors.New("sysctl kern.proc.all returned no processes")
	}

	out := make([]procInfo, 0, len(procs))
	for i := range procs {
		out = append(out, kinfoToProcInfo(&procs[i]))
	}
	return out, nil
}

// processComm returns the command name of one pid.
func processComm(pid int) (string, error) {
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", fmt.Errorf("sysctl kern.proc.pid %d: %w", pid, err)
	}
	return commString(proc.Proc.P_comm[:]), nil
}

// kinfoToProcInfo copies the three fields this package reads out of a kinfo_proc.
//
// The parent pid comes from Eproc.Ppid, not from Proc.P_oppid. P_oppid is the
// original parent, which the kernel fills in only while a debugger holds the
// process, so reading it would report almost every process as an orphan.
//
// P_comm is a bare name capped at 16 bytes, where `ps -o comm=` printed the
// full executable path. isClaudeComm matches on the suffix, so both forms hit.
func kinfoToProcInfo(p *unix.KinfoProc) procInfo {
	return procInfo{
		pid:  int(p.Proc.P_pid),
		ppid: int(p.Eproc.Ppid),
		comm: commString(p.Proc.P_comm[:]),
	}
}

// commString reads a fixed-width C string up to its NUL terminator.
func commString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// getProcessCwd returns the working directory of a process.
//
// macOS has no procfs. The supported call is proc_pidinfo in libproc, which
// needs cgo, and the release workflow cross-builds the darwin targets from a
// Linux runner in one job. So this asks lsof.
func getProcessCwd(pid int) (string, error) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}

	for _, line := range bytes.Split(out, []byte("\n")) {
		if !bytes.Contains(line, []byte(" cwd ")) {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) >= 9 {
			return string(fields[len(fields)-1]), nil
		}
	}
	return "", fmt.Errorf("cwd not found in lsof output for pid %d", pid)
}
