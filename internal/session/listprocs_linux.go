//go:build linux

package session

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// procRoot is where procfs is mounted. A var so a test can point the scan at a
// fixture tree instead of the machine's own process table.
var procRoot = "/proc"

// listProcessesNative reads the process table out of procfs.
//
// A pid that exits between the listing and the read is skipped. The table moves
// while it is being read, and one departed process is not a failed scan. A
// procfs that cannot be listed at all is a failed scan and is reported, because
// an empty list and a broken scan look identical downstream: every session
// would be marked inactive and csm would report no running sessions.
func listProcessesNative() ([]procInfo, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", procRoot, err)
	}

	procs := make([]procInfo, 0, len(entries))
	unreadable := 0
	for _, e := range entries {
		// procfs also holds named entries such as self, meminfo and net.
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "stat"))
		if err != nil {
			// A pid that is gone exited between the listing and the read,
			// which is normal. Anything else is a process that is still there
			// and cannot be seen, so it is counted rather than ignored.
			if !errors.Is(err, fs.ErrNotExist) {
				unreadable++
			}
			continue
		}
		comm, ppid, err := parseProcStat(data)
		if err != nil {
			unreadable++
			continue
		}
		procs = append(procs, procInfo{pid: pid, ppid: ppid, comm: comm})
	}

	// The scanning process is itself in procfs, so an empty result is a procfs
	// that is not what it claims to be, not a machine with nothing running. An
	// empty list downstream means "no Claude session is running", which csm
	// would then state with confidence.
	//
	// A partial failure still gets through. Under a hidepid mount csm reads its
	// own processes and not another user's, so this passes while that user's
	// claude sessions are invisible. getRunningClaudeDirs catches the narrower
	// case where claude processes were seen but none of them resolved.
	if len(procs) == 0 {
		if unreadable > 0 {
			return nil, fmt.Errorf("%s listed %d process entries and none could be read", procRoot, unreadable)
		}
		return nil, fmt.Errorf("%s holds no processes", procRoot)
	}
	return procs, nil
}

// processComm returns the command name of one pid.
func processComm(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	comm, _, err := parseProcStat(data)
	return comm, err
}

// parseProcStat pulls the command name and the parent pid out of one
// /proc/<pid>/stat line.
//
// The name is the second field and sits in parentheses. The kernel does not
// escape it, so a process called "foo) bar" holds both a space and a close
// paren. Splitting the line on whitespace, or cutting at the first ")", shifts
// every field after it and the parent pid then reads as some other number. That
// parent pid is the orphan signal, so a shift marks every session a ghost or
// none of them. Cutting at the last ")" is what keeps the fields lined up.
func parseProcStat(data []byte) (comm string, ppid int, err error) {
	start := bytes.IndexByte(data, '(')
	end := bytes.LastIndexByte(data, ')')
	if start < 0 || end < start {
		return "", 0, errors.New("no command field")
	}
	comm = string(data[start+1 : end])

	// After the name come the state and then the parent pid.
	fields := bytes.Fields(data[end+1:])
	if len(fields) < 2 {
		return "", 0, errors.New("no parent pid after the command field")
	}
	ppid, err = strconv.Atoi(string(fields[1]))
	if err != nil {
		return "", 0, fmt.Errorf("parent pid %q: %w", fields[1], err)
	}
	return comm, ppid, nil
}

// getProcessCwd returns the working directory of a process.
//
// Reading it needs the caller to be the same user as the target process, or
// root. Another user's process gives a permission error, and the caller skips
// that process.
func getProcessCwd(pid int) (string, error) {
	return os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "cwd"))
}
