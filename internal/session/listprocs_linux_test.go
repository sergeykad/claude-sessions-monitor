//go:build linux

package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The parent pid is the orphan signal. procfs does not escape the command name,
// so a name holding a space or a ")" shifts every field that follows it, and
// the parent pid then reads as some other number. Every session is then badged
// a ghost, or none of them is.
func TestParseProcStatKeepsThePPIDAlignedAfterAnOddCommandName(t *testing.T) {
	tests := []struct {
		name     string
		stat     string
		wantComm string
		wantPPID int
		wantErr  bool
	}{
		{
			name:     "plain name",
			stat:     "1868 (claude) S 487 1868 487 34816 2784 4194304 12345\n",
			wantComm: "claude",
			wantPPID: 487,
		},
		{
			name:     "name holding a close paren",
			stat:     "43 (weird) name) S 9 43 9 0 -1 4194560 99\n",
			wantComm: "weird) name",
			wantPPID: 9,
		},
		{
			name:    "no command field",
			stat:    "44 claude S 9 44\n",
			wantErr: true,
		},
		{
			name:    "cut off before the parent pid",
			stat:    "45 (claude) S\n",
			wantErr: true,
		},
		{
			name:    "parent pid is not a number",
			stat:    "46 (claude) S nope 46\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comm, ppid, err := parseProcStat([]byte(tt.stat))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsed %q as comm=%q ppid=%d; a malformed row must be reported", tt.stat, comm, ppid)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseProcStat(%q): %v", tt.stat, err)
			}
			if comm != tt.wantComm || ppid != tt.wantPPID {
				t.Errorf("got comm=%q ppid=%d, want comm=%q ppid=%d", comm, ppid, tt.wantComm, tt.wantPPID)
			}
		})
	}
}

// A process that exits between the directory listing and the read of its stat
// file must not take the scan down with it. If it did, a busy machine would
// intermittently report every Claude session as inactive.
func TestListProcessesNativeSkipsAProcessThatExitedMidScan(t *testing.T) {
	root := t.TempDir()
	writeProcEntry(t, root, "101", "101 (claude) S 1 101\n")
	// A pid directory with no stat file is what a process that has just exited
	// leaves behind for the moment between the listing and the read.
	if err := os.Mkdir(filepath.Join(root, "202"), 0o755); err != nil {
		t.Fatal(err)
	}
	// procfs also carries named entries, which are not processes. Real procfs
	// gives them a stat file too, so without one this fixture would be dropped
	// by the missing-stat check and prove nothing about the pid guard.
	selfDir := filepath.Join(root, "self")
	if err := os.Mkdir(selfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selfDir, "stat"), []byte("101 (claude) S 1 101\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProcEntry(t, root, "303", "303 (bash) S 101 303\n")

	procRootFor(t, root)
	procs, err := listProcessesNative()
	if err != nil {
		t.Fatalf("scan failed because one process was gone: %v", err)
	}

	// Keyed by pid rather than compared in order: the scan promises no order,
	// and asserting one would only pin os.ReadDir's sort.
	byPID := make(map[int]procInfo, len(procs))
	for _, p := range procs {
		byPID[p.pid] = p
	}
	for _, want := range []procInfo{
		{pid: 101, ppid: 1, comm: "claude"},
		{pid: 303, ppid: 101, comm: "bash"},
	} {
		got, ok := byPID[want.pid]
		if !ok {
			t.Errorf("pid %d is missing from the scan", want.pid)
			continue
		}
		if got != want {
			t.Errorf("pid %d = %+v, want %+v", want.pid, got, want)
		}
	}
	if _, ok := byPID[202]; ok {
		t.Error("the process that exited mid-scan is in the result")
	}
	if len(procs) != 2 {
		t.Errorf("got %d processes, want 2: %+v", len(procs), procs)
	}
}

// A procfs that cannot be listed is a broken scan, not an empty machine. A
// table that lists but yields nothing is rejected one level up, in
// getRunningClaudeDirs.
func TestListProcessesNativeReportsABrokenProcfs(t *testing.T) {
	procRootFor(t, filepath.Join(t.TempDir(), "no-such-procfs"))

	procs, err := listProcessesNative()
	if err == nil {
		t.Fatalf("scan returned %d processes and no error", len(procs))
	}
}

// The scan must read the fields procfs actually writes, which a hand-built
// fixture cannot prove. This checks it against the kernel's own output for the
// one process whose pid, parent and name the test already knows.
func TestListProcessesNativeFindsTheRunningTestProcess(t *testing.T) {
	procRootFor(t, "/proc")

	procs, err := listProcessesNative()
	if err != nil {
		t.Fatalf("scanning /proc: %v", err)
	}

	// The kernel publishes the same name in a file of its own, so this compares
	// what the parser pulled out of stat against what procfs says it is.
	wantComm, err := os.ReadFile("/proc/self/comm")
	if err != nil {
		t.Fatalf("reading /proc/self/comm: %v", err)
	}

	self := os.Getpid()
	for _, p := range procs {
		if p.pid != self {
			continue
		}
		if p.ppid != os.Getppid() {
			t.Errorf("ppid = %d, want %d", p.ppid, os.Getppid())
		}
		if got, want := p.comm, strings.TrimSpace(string(wantComm)); got != want {
			t.Errorf("comm = %q, want %q", got, want)
		}
		return
	}
	t.Errorf("the scan did not find this test process (pid %d) among %d processes", self, len(procs))
}

// procRootFor points the scan at a fixture tree for the duration of one test.
func procRootFor(t *testing.T, dir string) {
	t.Helper()
	original := procRoot
	t.Cleanup(func() { procRoot = original })
	procRoot = dir
}

// writeProcEntry creates <root>/<pid>/stat holding one procfs stat line.
func writeProcEntry(t *testing.T, root, pid, stat string) {
	t.Helper()
	if _, err := strconv.Atoi(pid); err != nil {
		t.Fatalf("fixture pid %q is not a number", pid)
	}
	dir := filepath.Join(root, pid)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatal(err)
	}
}

// isClaudeProcess is the guard --kill-ghosts runs before it sends SIGTERM. If
// it says yes for anything but a claude process, csm kills whatever unrelated
// process inherited a recycled pid.
func TestIsClaudeProcessRefusesAnythingButClaude(t *testing.T) {
	root := t.TempDir()
	writeProcEntry(t, root, "101", "101 (claude) S 1 101\n")
	writeProcEntry(t, root, "303", "303 (bash) S 1 303\n")
	procRootFor(t, root)

	tests := []struct {
		name string
		pid  int
		want bool
	}{
		{"a claude process", 101, true},
		{"an unrelated process holding a recycled pid", 303, false},
		{"a pid that no longer exists", 999, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClaudeProcess(tt.pid); got != tt.want {
				t.Errorf("isClaudeProcess(%d) = %v, want %v", tt.pid, got, tt.want)
			}
		})
	}
}
