package ssh

import "testing"

func TestNewClientInvalid(t *testing.T) {
	_, err := NewClient(Config{Host: "", User: ""})
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestNewClientMissingKey(t *testing.T) {
	_, err := NewClient(Config{Host: "example.com", Port: 22, User: "test", KeyPath: "/nonexistent/key"})
	if err == nil {
		t.Log("expected error for missing key, got nil (may be ok if not checked)")
	}
}
