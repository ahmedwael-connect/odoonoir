package service

import (
	"strings"
	"testing"

	"github.com/ahmed/odoonoir/internal/instance"
)

func TestTranslatePsqlError(t *testing.T) {
	tests := []struct {
		raw     string
		wantCode DBErrorCode
	}{
		{`database "foo" does not exist`, DBErrNotFound},
		{`role "odoo" does not exist`, DBErrAuth},
		{`password authentication failed for user "odoo"`, DBErrAuth},
		{`Peer authentication failed for user "odoo"`, DBErrAuth},
		{`could not connect to server: Connection refused`, DBErrConnRefused},
		{`no pg_hba.conf entry for host "127.0.0.1"`, DBErrAuth},
		{`database "x" is not served by instance "myapp"`, DBErrNotServed},
		{`invalid database name "Bad-Name"`, DBErrInvalidName},
		{`postgres is not accepting connections`, DBErrServerDown},
	}
	for _, tc := range tests {
		err := translatePsqlError(tc.raw, &DBError{Message: tc.raw})
		de, ok := err.(*DBError)
		if !ok {
			t.Fatalf("raw %q: expected DBError, got %T %v", tc.raw, err, err)
		}
		if de.Code != tc.wantCode {
			t.Fatalf("raw %q: want %s got %s hint %s", tc.raw, tc.wantCode, de.Code, de.Hint)
		}
		if !strings.Contains(de.Error(), "fix:") {
			t.Fatalf("expected hint in error for %q", tc.raw)
		}
	}
}

func TestTranslatePsqlErrorUnknown(t *testing.T) {
	err := translatePsqlError("some random psql failure", &DBError{Message: "random"})
	if de, ok := err.(*DBError); !ok || de.Code != DBErrUnknown {
		t.Fatalf("expected unknown, got %v", err)
	}
}

func TestResolveDBNoSelection(t *testing.T) {
	svc := newTestService(t)
	inst := &instance.Instance{Name: "app", Version: "18.0", DBName: "", Databases: []string{}}
	_ = svc.reg.Put(inst)
	// need instance with no DBName
	gotInst, _ := svc.reg.Get("app")
	if gotInst.DBName != "" {
		t.Fatal("expected empty DBName")
	}
	_, err := svc.ResolveDB(gotInst, "", true)
	if err == nil {
		t.Fatal("expected error for no database selected")
	}
	if de, ok := err.(*DBError); !ok || de.Code != DBErrNotServed {
		t.Fatalf("want NOT_SERVED, got %v", err)
	}
}

func TestValidateDBConfigLoadFail(t *testing.T) {
	svc := newTestService(t)
	// no instance
	_, err := svc.ValidateDBConfig("nope")
	if err == nil {
		t.Fatal("expected error for missing instance")
	}
}
