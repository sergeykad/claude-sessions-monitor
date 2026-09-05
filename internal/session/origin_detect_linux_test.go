//go:build linux

package session

import (
	"os"
	"path/filepath"
	"testing"
)

// parentChain walks ancestors through procRoot, name and parent pid from the
// stat file and the executable from the exe link. Reading /proc directly would
// leave this walk on the developer's real machine while the rest of the scan
// reads a fixture, and the origin of a session would be decided from whatever
// happened to be running.
func TestParentChainWalksFixtureProcfs(t *testing.T) {
	root := t.TempDir()
	procRootFor(t, root)

	// 42 is the session's process, parented to the terminal that launched it.
	writeProcEntry(t, root, "42", "42 (claude) S 7 7 7 0 -1 4194304 0 0 0 0 1 2 0 0 20 0 1 0 100 0 0\n")
	writeProcEntry(t, root, "7", "7 (ghostty) S 1 1 1 0 -1 4194304 0 0 0 0 1 2 0 0 20 0 1 0 100 0 0\n")
	if err := os.Symlink("/opt/ghostty/bin/ghostty", filepath.Join(root, "7", "exe")); err != nil {
		t.Fatal(err)
	}

	chain := parentChain(42)

	if len(chain) != 2 {
		t.Fatalf("walked %d ancestors, want 2: the origin column falls back to unknown when the chain breaks", len(chain))
	}
	if chain[0].PID != 42 || chain[0].Comm != "claude" {
		t.Errorf("first hop = pid %d %q, want pid 42 \"claude\"", chain[0].PID, chain[0].Comm)
	}
	if chain[1].PID != 7 {
		t.Errorf("second hop = pid %d, want 7: the parent pid was read from the wrong stat field", chain[1].PID)
	}
	if chain[1].Comm != "ghostty" {
		t.Errorf("second hop name = %q, want \"ghostty\"", chain[1].Comm)
	}
	// readExe reads through procRoot too. Without asserting it, this test passes
	// whether the link came from the fixture or the real machine.
	if chain[1].Exe != "/opt/ghostty/bin/ghostty" {
		t.Errorf("second hop Exe = %q, want the fixture's link target: origin detection names the terminal from this path",
			chain[1].Exe)
	}
}

// A process that vanishes mid-walk ends the chain rather than reporting an
// ancestor with no name.
func TestParentChainStopsWhenAnAncestorIsGone(t *testing.T) {
	root := t.TempDir()
	procRootFor(t, root)
	writeProcEntry(t, root, "42", "42 (claude) S 999 9 9 0 -1 4194304 0 0 0 0 1 2 0 0 20 0 1 0 100 0 0\n")

	chain := parentChain(42)

	if len(chain) != 1 {
		t.Fatalf("walked %d ancestors, want 1: pid 999 has no entry in the fixture", len(chain))
	}
	if chain[0].Comm != "claude" {
		t.Errorf("Comm = %q, want \"claude\"", chain[0].Comm)
	}
}

// readProcessEnv reads through procRoot too; without that it read the real
// machine's environ while the rest of the scan read the fixture.
func TestReadProcessEnvReadsFixtureProcfs(t *testing.T) {
	root := t.TempDir()
	procRootFor(t, root)
	writeProcEntry(t, root, "42", "42 (claude) S 1 1 1 0 -1 0 0 0 0 0 1 2 0 0 20 0 1 0 100 0 0\n")
	writeProcFile(t, root, "42", "environ", "TERM_PROGRAM=ghostty\x00SHLVL=2\x00")

	env := readProcessEnv(42)

	if env["TERM_PROGRAM"] != "ghostty" {
		t.Errorf("TERM_PROGRAM = %q, want \"ghostty\": origin detection reads this to name the terminal",
			env["TERM_PROGRAM"])
	}
	if env["SHLVL"] != "2" {
		t.Errorf("SHLVL = %q, want \"2\"", env["SHLVL"])
	}
}

// writeProcFile adds one file to an existing fixture <root>/<pid> directory.
func writeProcFile(t *testing.T, root, pid, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, pid, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
