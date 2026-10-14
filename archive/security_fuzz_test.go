package archive

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzManifestBinaryPath(f *testing.F) {
	for _, path := range []string{"bin/linux/hello", "../secret", `..\secret`, `C:\secret`, "/secret", ".", "", "bin/../secret", "bin/file:stream", "manifest.json", "bin/\x00file"} {
		f.Add(path)
	}
	f.Fuzz(func(t *testing.T, path string) {
		m := Manifest{ID: "hello", Version: "1.0.0", Binaries: map[string]Binary{"linux_amd64": {Path: path}}}
		err := m.Check()
		safe := fs.ValidPath(path) && path != "." && !strings.ContainsAny(path, "\\:\x00") && !strings.HasPrefix(path, "..") && path != ManifestName && path != SignatureName
		if (err == nil) != safe {
			t.Fatalf("path %q: err=%v safe=%v", path, err, safe)
		}
	})
}

func TestManifestPathRejectedBeforeSigningRead(t *testing.T) {
	dir := t.TempDir()
	raw := `{"id":"hello","version":"1.0.0","binaries":{"windows_amd64":{"path":"..\\outside.exe"}}}`
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := Sign(dir, "", &out); err == nil || !strings.Contains(err.Error(), "plain path inside") {
		t.Fatalf("expected manifest rejection before read, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("invalid manifest emitted archive data")
	}
}
