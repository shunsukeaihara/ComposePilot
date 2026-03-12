package gitops

import "testing"

func TestNormalizePrivateKey(t *testing.T) {
	got := normalizePrivateKey("  line1\r\nline2\rline3\n")
	want := "line1\nline2\nline3\n"
	if got != want {
		t.Fatalf("normalizePrivateKey() = %q, want %q", got, want)
	}
}
