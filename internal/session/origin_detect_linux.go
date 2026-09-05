//go:build linux

package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
)

// readProcessEnv returns the environment of a running process on Linux via
// <procRoot>/<pid>/environ. Only readable when csm runs as the same UID as the
// target process; returns an empty map on permission errors.
func readProcessEnv(pid int) map[string]string {
	env := make(map[string]string)
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "environ"))
	if err != nil {
		return env
	}
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		eq := bytes.IndexByte(entry, '=')
		if eq <= 0 {
			continue
		}
		env[string(entry[:eq])] = string(entry[eq+1:])
	}
	return env
}

// parentChain walks ancestors using <procRoot>/<pid>/stat for the name and
// parent pid, and <procRoot>/<pid>/exe for the executable path.
func parentChain(pid int) []ProcessInfo {
	var chain []ProcessInfo
	current := pid
	for hops := 0; hops < 10 && current > 1; hops++ {
		comm, ppid, err := readProcStat(current)
		if err != nil {
			return chain
		}
		exe := readExe(current)
		chain = append(chain, ProcessInfo{PID: current, Comm: comm, Exe: exe})
		if ppid <= 1 {
			return chain
		}
		current = ppid
	}
	return chain
}

// readProcStat returns a process's command name and parent pid. Both come from
// one read of the stat file, so they describe the same instant: reading the name
// and the parent separately lets the process exit in between and pairs a live
// parent pid with an empty name.
func readProcStat(pid int) (comm string, ppid int, err error) {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", 0, err
	}
	return parseProcStat(data)
}

func readExe(pid int) string {
	exe, err := os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "exe"))
	if err != nil {
		return ""
	}
	return exe
}
