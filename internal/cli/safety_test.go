package cli

import (
	"reflect"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestPassthroughFromArgs(t *testing.T) {
	cases := []struct {
		argv []string
		want []string
	}{
		{[]string{"odoonoir", "shell", "myapp"}, nil},
		{[]string{"odoonoir", "shell", "myapp", "--", "--no-http"}, []string{"--no-http"}},
		{[]string{"odoonoir", "shell", "myapp", "sales", "--", "-c", "x.conf", "--foo"}, []string{"-c", "x.conf", "--foo"}},
		{[]string{"odoonoir", "shell", "--", "--", "a"}, []string{"--", "a"}},
	}
	for _, tc := range cases {
		got := passthroughFromArgs(tc.argv)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("passthroughFromArgs(%v) = %v, want %v", tc.argv, got, tc.want)
		}
	}
}

func TestResolveDBNew(t *testing.T) {
	inst := &instance.Instance{Name: "myapp", DBName: "myapp", Databases: []string{"sales"}}
	cases := []struct {
		arg         string
		wantName    string
		wantTracked bool
		wantErr     bool
	}{
		{"", "myapp", true, false},
		{"sales", "sales", true, false},
		{"staging", "staging", false, false},
		{"Bad_Name", "", false, true},
	}
	for _, tc := range cases {
		name, tracked, err := resolveDBNew(inst, tc.arg)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("resolveDBNew(%q): expected error", tc.arg)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveDBNew(%q): %v", tc.arg, err)
		}
		if name != tc.wantName || tracked != tc.wantTracked {
			t.Fatalf("resolveDBNew(%q) = (%q, %v), want (%q, %v)", tc.arg, name, tracked, tc.wantName, tc.wantTracked)
		}
	}
}
