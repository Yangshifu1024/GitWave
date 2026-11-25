//go:build darwin || linux

package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDatabaseAndSidecarsAreOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		must(t, os.WriteFile(path+suffix, nil, 0o600))
		must(t, os.Chmod(path+suffix, 0o644))
	}
	must(t, protectDatabaseFiles(path))
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		info, err := os.Stat(path + suffix)
		must(t, err)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("state.db%s permissions = %o", suffix, info.Mode().Perm())
		}
	}
}
