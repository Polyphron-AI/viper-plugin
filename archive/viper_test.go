package archive

import "testing"

// TestRuns checks a manifest's Viper range against versions.
func TestRuns(t *testing.T) {
	cases := []struct {
		rng, version string
		runs, ok     bool
	}{
		{"", "v0.6.8", true, true},
		{"", "v0.7.0", false, true},
		{">=0.6.8 <0.7.0", "v0.6.7", false, true},
		{">=0.6.8 <0.7.0", "v0.6.8", true, true},
		{">=0.6.8 <0.7.0", "v0.6.9-3-gabc123", true, true},
		{"=0.6.8", "0.6.8", true, true},
		{">0.6.8", "v0.6.8", false, true},
		{"<=0.6.8", "v0.6.8", true, true},
		{">=0.6.8", "dev", false, false},
	}

	for _, c := range cases {
		runs, ok := Manifest{Viper: c.rng}.Runs(c.version)

		if runs != c.runs || ok != c.ok {
			t.Errorf(
				"%q on %s: got %v %v, want %v %v",
				c.rng,
				c.version,
				runs,
				ok,
				c.runs,
				c.ok,
			)
		}
	}

	for _, bad := range []string{"0.6", ">= 0.6.8", "~0.6.8", "latest"} {
		if _, err := parseRange(bad); err == nil {
			t.Errorf(
				"%q: want an error",
				bad,
			)
		}
	}
}
