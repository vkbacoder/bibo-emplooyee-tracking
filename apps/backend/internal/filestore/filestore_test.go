package filestore

import (
	"runtime"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Concurrent uploads of the same screenshot (a client retry racing the first
// attempt) must all succeed and leave exactly the final file, no temp files.
func TestWriteConcurrentSameScreenshot(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	const biz, user, shot = "7fecb849-60de-41bc-bd35-79fdf4a542f5", "b749aedc-8a9a-4a90-95df-e8d8969f9bcd", "ce96c893-9bb8-4cf4-945c-7922b273b9d2"
	data := bytes.Repeat([]byte("webp"), 10_000)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Write(biz, user, 1790442000, shot, data); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}

	rel, err := s.rel(biz, user, 1790442000, shot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("final file wrong: err=%v len=%d", err, len(got))
	}
	entries, _ := os.ReadDir(filepath.Dir(filepath.Join(root, rel)))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(root, rel)); info.Mode().Perm() != 0o644 {
			t.Errorf("mode = %v, want 0644", info.Mode().Perm())
		}
	}
}
