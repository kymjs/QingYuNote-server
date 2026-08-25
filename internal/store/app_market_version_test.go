package store

import "testing"

func TestSemanticVersions(t *testing.T) {
	if !IsSemanticVersion("1.1.17") || !IsSemanticVersion("1.1.17+42") {
		t.Fatal("expected valid semantic versions")
	}
	for _, value := range []string{"", "1..2", "one.two", "1.2.3-beta"} {
		if IsSemanticVersion(value) {
			t.Fatalf("expected invalid version %q", value)
		}
	}
	if CompareSemanticVersions("1.1.18", "1.1.17") <= 0 {
		t.Fatal("expected newer version")
	}
}
