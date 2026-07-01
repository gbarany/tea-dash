package data

import "testing"

func TestSplitOwnerRepo(t *testing.T) {
	owner, name, ok := SplitOwnerRepo("acme/widgets")
	if !ok || owner != "acme" || name != "widgets" {
		t.Fatalf(`SplitOwnerRepo("acme/widgets") = %q, %q, %v`, owner, name, ok)
	}

	for _, bad := range []string{"", "noslash", "a/b/c", "/x", "x/"} {
		if _, _, ok := SplitOwnerRepo(bad); ok {
			t.Fatalf("SplitOwnerRepo(%q) ok = true, want false", bad)
		}
	}
}
