package archive

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Command runs "keygen" or "sign" from args. name is how the caller is
// typed ("viper-plugin" or "viper plugin"), for the help text.
func Command(name string, args []string) error {
	usage := fmt.Errorf("usage: %s keygen [--out plugin.key] | %s sign [--key plugin.key] [--viper RANGE] [--out FILE] FOLDER", name, name)

	if len(args) == 0 {
		return usage
	}

	switch args[0] {
	case "keygen":
		return keygen(name, args[1:])
	case "sign":
		return sign(name, args[1:], usage)
	}

	return usage
}

// keygen writes a private signing key to a file and prints the public key,
// which goes into the server's VIPER_PLUGIN_KEYS.
func keygen(name string, args []string) error {
	fs := flag.NewFlagSet(
		name+" keygen",
		flag.ExitOnError,
	)
	out := fs.String(
		"out",
		"plugin.key",
		"file for the private key (keep it secret, never commit it)",
	)
	_ = fs.Parse(args)

	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s already exists; pick another --out", *out)
	}

	public, private, err := GenerateKey()
	if err != nil {
		return err
	}

	if err := os.WriteFile(
		*out,
		[]byte(private+"\n"),
		0o600,
	); err != nil {
		return err
	}

	fmt.Printf("Private key written to %s.\nPublic key (for VIPER_PLUGIN_KEYS):\n%s\n", *out, public)

	return nil
}

// sign turns a plugin folder into a signed .vplugin archive.
func sign(name string, args []string, usage error) error {
	fs := flag.NewFlagSet(
		name+" sign",
		flag.ExitOnError,
	)
	keyFile := fs.String(
		"key",
		"",
		"the private key file from keygen (leave out for an unsigned archive, which only a dev server takes)",
	)
	out := fs.String(
		"out",
		"",
		"the archive to write (default ID-VERSION.vplugin)",
	)
	viper := fs.String(
		"viper",
		"",
		"the Viper versions it runs on, like \">=0.6.8 <0.7.0\" (default: the manifest's, else any 0.6)",
	)
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		return usage
	}

	var key []byte

	if *keyFile != "" {
		var err error
		if key, err = os.ReadFile(*keyFile); err != nil {
			return err
		}
	}

	// Written next to where it goes, so the rename stays on one drive.
	tmp, err := os.CreateTemp(
		filepath.Dir(*out),
		"plugin-*.vplugin",
	)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	m, err := SignFor(
		fs.Arg(0),
		strings.TrimSpace(string(key)),
		*viper,
		tmp,
	)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return err
	}

	file := *out
	if file == "" {
		file = m.ID + "-" + m.Version + ".vplugin"
	}

	if err := os.Rename(
		tmp.Name(),
		file,
	); err != nil {
		return err
	}

	fmt.Printf("Signed %s %s for %d platforms into %s.\n", m.ID, m.Version, len(m.Binaries), file)

	return nil
}
