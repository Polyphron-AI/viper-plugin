package archive

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// AnyViper is the range a manifest without one gets: any 0.x release from
// 0.6 on. The plugin contract has not changed since 0.6, so a plugin that
// names no range runs until 1.0.
const AnyViper = ">=0.6.0 <1.0.0"

// rangePart is one comparison in a range, like ">=0.6.8".
var rangePart = regexp.MustCompile(`^(>=|<=|>|<|=)?(\d+\.\d+\.\d+)$`)

// releasePattern finds the release in a Viper version like "v0.6.8" or
// "v0.6.8-3-gabc123" (a build between releases counts as that release).
var releasePattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// ViperRange is the manifest's Viper range, or AnyViper when it has none.
func (m Manifest) ViperRange() string {
	if strings.TrimSpace(m.Viper) == "" {
		return AnyViper
	}

	return m.Viper
}

// Runs says whether the manifest's plugin may run on a Viper version. ok is
// false when version isn't a release (a dev build), so the caller skips
// the check.
func (m Manifest) Runs(version string) (runs, ok bool) {
	v, ok := release(version)
	if !ok {
		return false, false
	}

	parts, err := parseRange(m.ViperRange())
	if err != nil {
		return false, true
	}

	for _, p := range parts {
		if !p.holds(v) {
			return false, true
		}
	}

	return true, true
}

// comparison is one part of a range.
type comparison struct {
	op string
	v  [3]int
}

// holds says whether v passes the comparison.
func (c comparison) holds(v [3]int) bool {
	n := slices.Compare(
		v[:],
		c.v[:],
	)

	switch c.op {
	case ">=":
		return n >= 0
	case "<=":
		return n <= 0
	case ">":
		return n > 0
	case "<":
		return n < 0
	}

	return n == 0
}

// parseRange reads a range: comparisons split by spaces, all of which must
// hold, like ">=0.6.8 <1.0.0".
func parseRange(text string) ([]comparison, error) {
	var out []comparison

	for _, f := range strings.Fields(text) {
		m := rangePart.FindStringSubmatch(f)
		if m == nil {
			return nil, fmt.Errorf("the viper range %q must be comparisons like \">=0.6.8 <1.0.0\"", text)
		}

		v, _ := release(m[2])
		out = append(
			out,
			comparison{op: m[1], v: v},
		)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("the viper range %q is empty", text)
	}

	return out, nil
}

// release reads major, minor and patch from a version.
func release(version string) ([3]int, bool) {
	var v [3]int

	m := releasePattern.FindStringSubmatch(strings.TrimSpace(version))
	if m == nil {
		return v, false
	}

	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}

	return v, true
}
