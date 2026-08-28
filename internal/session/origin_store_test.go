package session

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOriginStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	originStoreDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { originStoreDirFn = defaultOriginStoreDir })

	sid := "d3adbeef-0000-1111-2222-aaaabbbbcccc"
	want := Origin{Category: OriginTerminal, App: "ghostty", Display: "Ghostty"}

	if err := SaveOrigin(sid, want); err != nil {
		t.Fatalf("SaveOrigin: %v", err)
	}

	got, ok := LoadOrigin(sid)
	if !ok {
		t.Fatalf("LoadOrigin returned ok=false after save")
	}
	if got != want {
		t.Errorf("LoadOrigin = %+v, want %+v", got, want)
	}

	// File must exist at the expected path.
	if _, err := filepath.Abs(filepath.Join(dir, sid+".json")); err != nil {
		t.Fatalf("expected file path unreachable: %v", err)
	}
}

func TestOriginStoreSkipsEmpty(t *testing.T) {
	dir := t.TempDir()
	originStoreDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { originStoreDirFn = defaultOriginStoreDir })

	// Zero origin should be a no-op.
	if err := SaveOrigin("sid", Origin{}); err != nil {
		t.Fatalf("SaveOrigin with zero origin: %v", err)
	}
	if _, ok := LoadOrigin("sid"); ok {
		t.Fatalf("LoadOrigin should return ok=false for never-saved id")
	}

	// Empty session id should be a no-op.
	if err := SaveOrigin("", Origin{Category: OriginTerminal, App: "ghostty", Display: "Ghostty"}); err != nil {
		t.Fatalf("SaveOrigin with empty sid: %v", err)
	}
}

func TestLoadOriginMissing(t *testing.T) {
	dir := t.TempDir()
	originStoreDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { originStoreDirFn = defaultOriginStoreDir })

	if _, ok := LoadOrigin("no-such-id"); ok {
		t.Errorf("LoadOrigin of missing id should return ok=false")
	}
}

// A store directory other accounts can read hands them the id of every session
// the user has open: the store is one file per session, named by session id.
func TestSaveOriginKeepsTheStoreDirectoryPrivate(t *testing.T) {
	// umask is process-global and would strip the very bits this test looks
	// for, making the result depend on whoever ran it.
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	// A filesystem that drops mode bits would fail this for a reason that has
	// nothing to do with the code under test.
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatalf("probe mkdir: %v", err)
	}
	if info, err := os.Stat(probe); err != nil || info.Mode().Perm() != 0o755 {
		t.Skip("filesystem does not preserve directory permissions")
	}

	tests := []struct {
		name     string
		existing os.FileMode // zero when the store does not exist yet
	}{
		{"store created by this save", 0},
		{"store an earlier version left readable", 0o755},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "origins")
			if tt.existing != 0 {
				if err := os.Mkdir(dir, tt.existing); err != nil {
					t.Fatalf("pre-create store dir: %v", err)
				}
			}
			originStoreDirFn = func() (string, error) { return dir, nil }
			t.Cleanup(func() { originStoreDirFn = defaultOriginStoreDir })

			sid := "d3adbeef-0000-1111-2222-aaaabbbbcccc"
			if err := SaveOrigin(sid, Origin{Category: OriginTerminal, App: "ghostty", Display: "Ghostty"}); err != nil {
				t.Fatalf("SaveOrigin: %v", err)
			}

			info, err := os.Stat(dir)
			if err != nil {
				t.Fatalf("stat store dir: %v", err)
			}
			if perm := info.Mode().Perm(); perm&0o077 != 0 {
				t.Errorf("store dir is %04o; group and other must have no access at all", perm)
			}
		})
	}
}
