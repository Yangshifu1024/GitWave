package gitcli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMoveSnapshotBuffersAndSpills(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget int64
		spills bool
	}{
		{"materialize on demand", 8, false},
		{"spill when buffers exceed the budget", 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			snapshot := moveSnapshotFiles{root: root, remaining: tc.budget}
			defer snapshot.close()
			files := map[string]string{"first.txt": "one", "second.txt": "two"}
			for name, content := range files {
				data := make([]byte, len(content), 4)
				copy(data, content)
				if err := snapshot.add(filepath.Join("nested", name), workFileContent{data: data, mode: 0o600}); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(root.Name())
			if err != nil || (len(entries) != 0) != tc.spills {
				t.Fatalf("unexpected private worktree contents before materialization: %v, %v", entries, err)
			}
			if err := snapshot.materialize(); err != nil {
				t.Fatal(err)
			}
			for name, content := range files {
				if data, err := root.ReadFile(filepath.Join("nested", name)); err != nil || string(data) != content {
					t.Fatalf("snapshot of %s = %q, %v", name, data, err)
				}
			}
		})
	}
}

func TestMoveSnapshotUsesMetadataFromTheOpenFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this fixture checks the POSIX executable bit")
	}
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	path := filepath.Join(dir, "candidate")
	if err := os.WriteFile(path, []byte("move contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	initial, err := root.Lstat("candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := readMoveFile(t.Context(), root, "candidate", maxMoveFileBytes, initial)
	if err != nil || file.moveSignature().mode&0o100 == 0 {
		t.Fatalf("expected the open file's executable bit, mode %v, error %v", file.mode, err)
	}
	if err := os.Rename(path, filepath.Join(dir, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readMoveFile(t.Context(), root, "candidate", maxMoveFileBytes, initial); err == nil {
		t.Fatal("accepted a replacement that differed from the initially inspected file")
	}
}
