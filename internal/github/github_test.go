package github

import "testing"

func TestNewFileTokenStore(t *testing.T) {
	s, err := NewFileTokenStore()
	if err != nil {
		t.Fatal(err)
	}
	// Get should not error and return empty when no file
	tok, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	_ = tok
}

func TestNewClient(t *testing.T) {
	c := NewClient("dummy")
	if c == nil || c.GoClient() == nil {
		t.Fatal("client nil")
	}
}
