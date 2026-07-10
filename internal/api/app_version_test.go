package api

import "testing"

func TestCompareSemanticVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.1.11", "1.1.8", 1},
		{"1.2.3+4", "1.2.3", 0},
		{"", "1.0.0", -1},
	}
	for _, tc := range cases {
		got := compareSemanticVersions(tc.a, tc.b)
		if got != tc.want {
			t.Fatalf("compareSemanticVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
