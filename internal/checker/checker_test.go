package checker

import "testing"

func TestPythonCompatRanges(t *testing.T) {
	if _, ok := PythonCompat["17"]; !ok {
		t.Fatal("17 missing")
	}
	if _, ok := PythonCompat["18"]; !ok {
		t.Fatal("18 missing")
	}
	if _, ok := PythonCompat["19"]; !ok {
		t.Fatal("19 missing")
	}
	for v, pys := range PythonCompat {
		if len(pys) == 0 {
			t.Errorf("empty python range for %s", v)
		}
	}
}

func TestPortInUse(t *testing.T) {
	if PortInUse(0) {
		t.Error("port 0 should be free")
	}
}

func TestRunDoesNotPanic(t *testing.T) {
	c := Run()
	if len(c.Results) == 0 {
		t.Fatal("no results")
	}
}
