package archive

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSignUnsigned checks sign without --key writes an archive: an
// unsigned one, the kind a dev server takes.
func TestSignUnsigned(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(
		filepath.Join(
			dir,
			"bin",
		),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	for name, body := range map[string]string{
		"manifest.json": `{"id":"hello","version":"1.0.0","title":"Hello","binaries":{"linux/amd64":{"path":"bin/hello"}}}`,
		"bin/hello":     "binary",
	} {
		if err := os.WriteFile(
			filepath.Join(
				dir,
				name,
			),
			[]byte(body),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(
		t.TempDir(),
		"hello.vplugin",
	)

	if err := Command(
		"viper-plugin",
		[]string{"sign", "--out", out, dir},
	); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Fatalf(
			"no archive: %v",
			err,
		)
	}
}
