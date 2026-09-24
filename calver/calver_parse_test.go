package calver

import "testing"

// TestCompareCalVer pins the component-wise CalVer comparison, including the
// TWO-TAG-ENCODING normalization. The org's Go-module repos (sdk, spec,
// plugin-gh) tag `v0.<YYYYDDD>.<HHMM>` while every other repo tags the plain
// `v<YYYY>.<DDD>.<HHMM>`; a repo mid-migration carries BOTH, and before the
// normalization the Go form's major `0` ranked BELOW the plain form's `2026`,
// so a STALE plain tag won over a NEWER Go-form tag (the docs `extra_repos`
// resolution bug: plugin-gh `v2026.252.1501` beat `v0.2026266.2326`).
func TestCompareCalVer(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		// Plain CalVer, both bare and v-prefixed.
		{"plain equal", "2026.240.1944", "2026.240.1944", 0},
		{"plain newer", "2026.240.1945", "2026.240.1944", 1},
		{"plain older", "2026.240.1944", "2026.240.1945", -1},
		{"plain v-prefixed", "v2026.240.1945", "v2026.240.1944", 1},

		// Genuine semver stays component-wise (the multi-digit-major case).
		{"semver patch", "v1.2.3", "v1.2.4", -1},
		{"semver multi-digit major", "v1.2.3", "v10.0.0", -1},
		{"semver equal", "v1.0.0", "v1.0.0", 0},

		// Go-module tag form vs plain form: the two encodings share ONE timeline.
		{"go newer than plain", "v0.2026266.2326", "v2026.252.1501", 1},
		{"plain older than go", "v2026.252.1501", "v0.2026266.2326", -1},
		{"spec go newer than its last plain", "v0.2026266.2126", "v2026.240.1944", 1},
		{"sdk go newer than its last plain", "v0.2026266.2340", "v2026.232.1949", 1},
		{"same-tuple across encodings equal", "v0.2026266.2326", "v2026.266.2326", 0},

		// A small genuine semver major 0 stays below a CalVer year — correct:
		// it is NOT the Go-module 7-digit form, so it is not normalized.
		{"bare v0.1.2 is not a CalVer tag", "v0.1.2", "v2026.266.2326", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareCalVer(tt.a, tt.b); got != tt.want {
				t.Errorf("CompareCalVer(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			// Anti-symmetry: the reverse must negate.
			if got := CompareCalVer(tt.b, tt.a); got != -tt.want {
				t.Errorf("CompareCalVer(%q, %q) = %d, want %d (anti-symmetry)", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

// TestNormalizeCalVer pins the encoding-normalization boundary: ONLY the exact
// Go-module form `0.<7 digits>.<all digits>` is rewritten to canonical CalVer;
// everything else passes through byte-identical.
func TestNormalizeCalVer(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0.2026266.2326", "2026.266.2326"},            // Go form, zero-stripped HHMM
		{"0.2026266.0623", "2026.266.0623"},            // Go form, already-padded HHMM
		{"0.2026001.5", "2026.001.0005"},               // day + HHMM re-padded canonically
		{"2026.240.1944", "2026.240.1944"},             // plain CalVer: unchanged
		{"1.2.3", "1.2.3"},                             // semver: unchanged
		{"0.1.2", "0.1.2"},                             // major 0 but not the 7-digit form
		{"0.202626.2326", "0.202626.2326"},             // 6-digit second: not the form
		{"0.2026266.x", "0.2026266.x"},                 // non-digit HHMM: not the form
		{"0.2026266.", "0.2026266."},                   // empty HHMM: not the form
		{"2026.266.2326.extra", "2026.266.2326.extra"}, // 4 parts: unchanged
	}
	for _, tt := range tests {
		if got := normalizeCalVer(tt.in); got != tt.want {
			t.Errorf("normalizeCalVer(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
