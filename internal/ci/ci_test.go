package ci

import "testing"

func TestNewCIServiceNil(t *testing.T) {
	s := NewCIService(nil)
	if s == nil {
		t.Fatal("expected service")
	}
	// Don't call ListWorkflows with nil client — it panics; just verify service creation
}
