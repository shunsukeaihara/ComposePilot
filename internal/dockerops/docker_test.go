package dockerops

import "testing"

func TestBuildComposeArgs(t *testing.T) {
	got := buildComposeArgs([]string{"docker-compose.yml", "docker-compose.prod.yml"}, "up", "-d")
	want := []string{"compose", "-f", "docker-compose.yml", "-f", "docker-compose.prod.yml", "up", "-d"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
