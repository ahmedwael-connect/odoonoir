package installer

import "testing"

func TestExtraPackagesFor(t *testing.T) {
	if got := extraPackagesFor("16"); len(got) != 1 || got[0] != "lxml_html_clean==0.2.2" {
		t.Fatalf("extra for 16: %+v", got)
	}
	if got := extraPackagesFor("17"); len(got) != 1 {
		t.Fatalf("extra for 17: %+v", got)
	}
	if got := extraPackagesFor("18"); got != nil {
		t.Fatalf("extra for 18 should be nil, got %+v", got)
	}
	if got := extraPackagesFor("15"); len(got) != 1 {
		t.Fatalf("extra for 15: %+v", got)
	}
}

func TestSetuptoolsSpecFor(t *testing.T) {
	if got := setuptoolsSpecFor("16"); got != "setuptools<81" {
		t.Fatalf("16: %q", got)
	}
	if got := setuptoolsSpecFor("17"); got != "setuptools<81" {
		t.Fatalf("17: %q", got)
	}
	if got := setuptoolsSpecFor("18"); got != "setuptools" {
		t.Fatalf("18: %q", got)
	}
}

func TestResolvePython16(t *testing.T) {
	// Just verify it returns something without panic; actual binary may not exist on all CI
	_ = ResolvePython("16")
	_ = ResolvePython("15")
	_ = ResolvePython("18")
}

func TestBranchFor(t *testing.T) {
	if got := BranchFor("16"); got != "16.0" {
		t.Fatalf("branch 16: %q", got)
	}
	if got := BranchFor("18"); got != "18.0" {
		t.Fatalf("branch 18: %q", got)
	}
}
