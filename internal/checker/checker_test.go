package checker

import "testing"

func TestPythonCompatRanges(t *testing.T) {
	for _, v := range []string{"16", "17", "18", "19"} {
		if _, ok := PythonCompat[v]; !ok {
			t.Fatalf("%s missing", v)
		}
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
