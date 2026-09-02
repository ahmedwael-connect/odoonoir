package prompt

import "testing"

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b"}, "a") {
		t.Fatal("contains a")
	}
	if contains([]string{"a", "b"}, "c") {
		t.Fatal("not contains c")
	}
}
