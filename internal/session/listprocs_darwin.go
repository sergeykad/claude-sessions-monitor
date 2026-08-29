//go:build darwin

package session

import (
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
	// A scan without lsof is useless: every working directory below would fail
	// to resolve, leaving an empty map that reads downstream as a machine with
	// no Claude session running. Checked once here, where it is a property of
	// the scan, rather than inferred from a run of per-process failures.
	if _, err := exec.LookPath("lsof"); err != nil {
		return nil, fmt.Errorf("lsof is needed to read a process working directory: %w", err)
	}

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
	return unix.ByteSliceToString(proc.Proc.P_comm[:]), nil
}

// kinfoToProcInfo copies the three fields this package reads out of a kinfo_proc.
//
// The parent pid comes from Eproc.Ppid, not from Proc.P_oppid. P_oppid is the
// parent saved while a debugger holds the process and is zero otherwise, so the
// orphan test (ppid == 1) would never fire and no macOS session would ever be
// badged a ghost.
//
// P_comm is the accounting name: the executable's basename, capped at 16 bytes.
// isClaudeComm matches on the suffix.
func kinfoToProcInfo(p *unix.KinfoProc) procInfo {
	return procInfo{
		pid:  int(p.Proc.P_pid),
		ppid: int(p.Eproc.Ppid),
		comm: unix.ByteSliceToString(p.Proc.P_comm[:]),
	}
}

// getProcessCwd returns the working directory of a process.
//
// macOS has no procfs. The supported call is proc_pidinfo in libproc, which
// needs cgo, and the release workflow cross-builds the darwin targets from a
// Linux runner in one job. So this asks lsof.
func getProcessCwd(pid int) (string, error) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid), "-a", "-d", "cwd", "-Fn").Output()
	if err != nil {
		return "", err
	}

	cwd, err := parseLsofCwd(out)
	if err != nil {
		return "", fmt.Errorf("pid %d: %w", pid, err)
	}
	return cwd, nil
}
