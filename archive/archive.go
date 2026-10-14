// Package archive builds and signs Viper plugin archives (.vplugin files).
// An archive is a zip holding manifest.json, an optional manifest.sig (an
// Ed25519 signature of the manifest) and one binary per OS and arch. Viper
// checks the same rules again when an archive is installed.
package archive

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// Names inside an archive.
const (
	ManifestName  = "manifest.json"
	SignatureName = "manifest.sig"
)

// MaxBinarySize caps one unpacked binary, so a crafted zip can't fill a disk.
const MaxBinarySize int64 = 200 << 20

// idPattern is a plugin id: lowercase letters and digits, so block types
// ("<id>.<name>") and tool names ("<id>_<name>") split one way only.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]{1,30}$`)

// versionPattern is a semver version.
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$`)

// RouteMethods are the methods a plugin route may answer.
var RouteMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// ElevationActions are the actions an elevation may grant. file is saving
// a file on a record, which also needs update (it goes through that rule).
var ElevationActions = []string{"list", "view", "create", "update", "delete", "file"}

// MaxElevations caps a manifest's elevated routes.
const MaxElevations = 32

// Binary is one program in an archive, for one OS and arch.
type Binary struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Manifest is the signed part of an archive.
type Manifest struct {
	ID       string            `json:"id"`
	Version  string            `json:"version"`
	Title    string            `json:"title"`
	Binaries map[string]Binary `json:"binaries"`

	// Viper is the range of Viper versions the plugin runs on, like
	// ">=0.6.8 <1.0.0". Empty means any 0.x release from 0.6 on (AnyViper).
	Viper string `json:"viper,omitempty"`

	// Elevated lists the app routes that run as the service, and what each
	// may reach. It is signed, so a plugin can't widen it at runtime; the
	// route must also set Route.elevated in its description.
	Elevated []Elevation `json:"elevated,omitempty"`

	// Connections lists the connected accounts the plugin may ask a token
	// for (Host.GetAccessToken), and with which scopes. A project accepts
	// them when it turns the plugin on, like elevated routes.
	Connections []Connection `json:"connections,omitempty"`
}

// Connection lets a plugin ask for a user's token at Provider ("google" or
// "microsoft") with some of Scopes. Background lets a job or event ask for
// a user its payload names; without it only that user's own calls may.
type Connection struct {
	Provider   string   `json:"provider"`
	Scopes     []string `json:"scopes"`
	Background bool     `json:"background,omitempty"`
}

// ConnectionProviders are the providers a plugin can connect to.
var ConnectionProviders = []string{"google", "microsoft"}

// Elevation grants one app route the service identity on some collections
// and actions. Route is "<METHOD> <path>", as the plugin declares it.
type Elevation struct {
	Route       string   `json:"route"`
	Collections []string `json:"collections"`
	Actions     []string `json:"actions"`
}

// Platform is this machine's "<os>/<arch>", the key a binary goes under.
func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// GenerateKey makes a signing key pair, both base64.
func GenerateKey() (public, private string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(pub),
		base64.StdEncoding.EncodeToString(priv),
		nil
}

// Sign builds a signed archive from a folder holding manifest.json and the
// binaries it names. It fills in each binary's sha256. An empty private key
// makes an unsigned archive, which only a dev server takes.
func Sign(dir, privateKey string, w io.Writer) (Manifest, error) {
	return SignFor(
		dir,
		privateKey,
		"",
		w,
	)
}

// SignFor is Sign with the Viper range set; an empty viper keeps the
// manifest's.
func SignFor(dir, privateKey, viper string, w io.Writer) (Manifest, error) {
	var m Manifest

	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return m, err
	}

	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("manifest.json: %w", err)
	}

	if viper != "" {
		m.Viper = viper
	}

	if err := m.Check(); err != nil {
		return m, err
	}

	for platform, b := range m.Binaries {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(b.Path)))
		if err != nil {
			return m, err
		}

		sum := sha256.Sum256(data)
		b.SHA256 = hex.EncodeToString(sum[:])
		m.Binaries[platform] = b
	}

	signed, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}

	zw := zip.NewWriter(w)

	if err := add(zw, ManifestName, signed); err != nil {
		return m, err
	}

	if privateKey != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateKey))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return m, fmt.Errorf("the private key is not a base64 Ed25519 key")
		}

		sig := ed25519.Sign(ed25519.PrivateKey(key), signed)

		if err := add(zw, SignatureName, []byte(base64.StdEncoding.EncodeToString(sig))); err != nil {
			return m, err
		}
	}

	for _, platform := range m.Platforms() {
		b := m.Binaries[platform]

		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(b.Path)))
		if err != nil {
			return m, err
		}

		if err := add(zw, b.Path, data); err != nil {
			return m, err
		}
	}

	return m, zw.Close()
}

// add writes one file into a zip.
func add(zw *zip.Writer, name string, data []byte) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}

	_, err = f.Write(data)

	return err
}

// Check refuses a manifest Viper can't use safely.
func (m Manifest) Check() error {
	if !idPattern.MatchString(m.ID) {
		return fmt.Errorf("the plugin id %q must be 2 to 31 lowercase letters and digits, starting with a letter", m.ID)
	}

	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("the plugin version %q must be a semver version, like 1.2.0", m.Version)
	}

	if m.Viper != "" {
		if _, err := parseRange(m.Viper); err != nil {
			return err
		}
	}

	if len(m.Binaries) == 0 {
		return fmt.Errorf("the manifest lists no binaries")
	}

	for platform, b := range m.Binaries {
		clean := path.Clean(b.Path)

		if !fs.ValidPath(b.Path) || b.Path == "." || strings.ContainsAny(b.Path, "\\:\x00") || clean != b.Path || path.IsAbs(clean) || strings.HasPrefix(clean, "..") || clean == ManifestName || clean == SignatureName {
			return fmt.Errorf("the binary path %q for %s must be a plain path inside the archive", b.Path, platform)
		}
	}

	if len(m.Elevated) > MaxElevations {
		return fmt.Errorf("the manifest elevates %d routes; the most is %d", len(m.Elevated), MaxElevations)
	}

	seen := map[string]bool{}

	for _, e := range m.Elevated {
		method, path, _ := strings.Cut(e.Route, " ")

		if !slices.Contains(RouteMethods, method) || path == "" {
			return fmt.Errorf("the elevated route %q must be a method and a path, like \"POST items\"", e.Route)
		}

		if seen[e.Route] {
			return fmt.Errorf("the manifest elevates the route %q twice", e.Route)
		}

		seen[e.Route] = true

		if len(e.Collections) == 0 {
			return fmt.Errorf("the elevated route %q names no collections", e.Route)
		}

		if len(e.Actions) == 0 {
			return fmt.Errorf("the elevated route %q names no actions", e.Route)
		}

		for _, a := range e.Actions {
			if !slices.Contains(ElevationActions, a) {
				return fmt.Errorf("the elevated route %q has the action %q; use %s", e.Route, a, strings.Join(ElevationActions, ", "))
			}
		}
	}

	return m.checkConnections()
}

// checkConnections refuses a connection Viper can't grant.
func (m Manifest) checkConnections() error {
	seen := map[string]bool{}

	for _, c := range m.Connections {
		if !slices.Contains(ConnectionProviders, c.Provider) {
			return fmt.Errorf("the connection %q must be one of %s", c.Provider, strings.Join(ConnectionProviders, ", "))
		}

		if seen[c.Provider] {
			return fmt.Errorf("the manifest names the connection %q twice", c.Provider)
		}

		seen[c.Provider] = true

		if len(c.Scopes) == 0 {
			return fmt.Errorf("the connection %q names no scopes", c.Provider)
		}

		for _, scope := range c.Scopes {
			if len(strings.Fields(scope)) != 1 {
				return fmt.Errorf("the connection %q has the scope %q; each scope is one word", c.Provider, scope)
			}
		}
	}

	return nil
}

// Platforms lists the manifest's platforms in a fixed order.
func (m Manifest) Platforms() []string {
	out := make([]string, 0, len(m.Binaries))

	for p := range m.Binaries {
		out = append(out, p)
	}

	sort.Strings(out)

	return out
}
