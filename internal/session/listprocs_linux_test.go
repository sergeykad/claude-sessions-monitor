//go:build linux

package session

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// A procfs that cannot be read, or that holds no processes at all, is a broken
// scan and not an empty machine: the scanning process is itself in there. If
// either came back as an empty list, csm would print "No active Claude
// sessions." with full confidence while sessions ran.
func TestListProcessesNativeReportsABrokenProcfs(t *testing.T) {
	tests := []struct {
		name string
		root func(t *testing.T) string
	}{
		{
			name: "procfs is not there",
			root: func(t *testing.T) string { return filepath.Join(t.TempDir(), "no-such-procfs") },
		},
		{
			name: "procfs lists no processes",
			root: func(t *testing.T) string { return t.TempDir() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procRootFor(t, tt.root(t))

			procs, err := listProcessesNative()
			if err == nil {
				t.Fatalf("scan returned %d processes and no error", len(procs))
			}
		})
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

// The orphan set, the claude filter and the cwd-to-project mapping are what
// the dashboard reads. A process filtered wrongly vanishes from the dashboard,
// and an orphan flag read from the wrong field badges every session a ghost or
// none of them.
func TestGetRunningClaudeDirsFiltersAndFlagsWhatTheDashboardShows(t *testing.T) {
	root := t.TempDir()
	// 101: a claude whose parent is gone. 202: a claude with a live parent.
	// 303: not claude at all. All three share a working directory.
	writeProcEntry(t, root, "101", "101 (claude) S 1 101\n")
	writeProcEntry(t, root, "202", "202 (claude) S 900 202\n")
	writeProcEntry(t, root, "303", "303 (bash) S 1 303\n")
	for _, pid := range []string{"101", "202", "303"} {
		// os.Readlink does not resolve the target, so it need not exist.
		if err := os.Symlink("/home/u/proj", filepath.Join(root, pid, "cwd")); err != nil {
			t.Fatal(err)
		}
	}
	procRootFor(t, root)

	dirs, orphaned, err := getRunningClaudeDirs()
	if err != nil {
		t.Fatalf("getRunningClaudeDirs: %v", err)
	}

	encoded := encodeProjectPath("/home/u/proj")
	got := dirs[encoded]
	sort.Ints(got)
	if len(got) != 2 || got[0] != 101 || got[1] != 202 {
		t.Errorf("pids for %s = %v, want [101 202]; bash must not be counted as a session", encoded, got)
	}
	if len(dirs) != 1 {
		t.Errorf("got %d project keys, want 1: %v", len(dirs), dirs)
	}
	if !orphaned[101] {
		t.Error("the claude whose parent is gone is not flagged an orphan, so it can never be badged a ghost")
	}
	if orphaned[202] {
		t.Error("a claude with a live parent is flagged an orphan, which badges a session left open overnight as a ghost")
	}
}

// A cwd lookup that is unavailable, which on macOS is lsof missing from PATH,
// leaves every process out of the map. Returned as an empty map it reads as
// "no Claude session is running", which csm would then print while sessions ran.
//
// A process merely refusing the read is the opposite case and must stay silent:
// on a shared machine another user's claude is not a fault, and an error on
// screen would be wrong for a machine where "no sessions of yours" is true.
func TestGetRunningClaudeDirsSeparatesABrokenLookupFromARefusedOne(t *testing.T) {
	tests := []struct {
		name       string
		cwdErr     error
		wantReport bool
	}{
		{"the lookup tool is not installed", fmt.Errorf("%w: exec: lsof: not found", errCwdLookupBroken), true},
		{"another user's process refuses the read", fs.ErrPermission, false},
		{"the process exited mid-scan", fs.ErrNotExist, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeProcEntry(t, root, "101", "101 (claude) S 1 101\n")
			procRootFor(t, root)

			original := getProcessCwdFn
			t.Cleanup(func() { getProcessCwdFn = original })
			getProcessCwdFn = func(int) (string, error) { return "", tt.cwdErr }

			dirs, _, err := getRunningClaudeDirs()
			if tt.wantReport && err == nil {
				t.Fatalf("a broken lookup gave dirs=%v and no error", dirs)
			}
			if !tt.wantReport && err != nil {
				t.Fatalf("a process refusing the read was reported as a failure: %v", err)
			}
		})
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
