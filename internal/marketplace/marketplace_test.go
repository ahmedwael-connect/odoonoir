package marketplace

import (
	"path/filepath"
	"testing"
)

func TestFileStoreCreateAndSearch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, nil)
	mod := &MarketplaceModule{
		ID: "test_mod", Owner: "myorg", Repo: "my_module", Name: "my_module", DisplayName: "My Module",
		Summary: "test", Author: "tester", License: "LGPL-3", Category: "Tools", Version: "18.0.1.0.0",
		Stars: 10,
	}
	if err := store.SaveModule(mod); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetModule("test_mod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "my_module" {
		t.Fatalf("got %q", got.Name)
	}
	// search
	res, err := svc.SearchModules("my", ModuleFilter{PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatalf("expected search results")
	}
	// stats
	stats, err := svc.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalModules != 1 {
		t.Fatalf("stats total %d", stats.TotalModules)
	}
}

func TestModuleFilter(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(filepath.Join(dir, "s"))
	svc := NewService(store, nil)
	for _, m := range []*MarketplaceModule{
		{ID: "a", Name: "a", Category: "Tools", Stars: 5, Verified: true},
		{ID: "b", Name: "b", Category: "Sale", Stars: 20, Verified: false, Featured: true},
	} {
		_ = store.SaveModule(m)
	}
	verified, _ := svc.GetVerifiedModules(10)
	if len(verified) != 1 || verified[0].ID != "a" {
		t.Fatalf("verified %+v", verified)
	}
	featured, _ := svc.GetFeaturedModules(10)
	if len(featured) != 1 || featured[0].ID != "b" {
		t.Fatalf("featured %+v", featured)
	}
}
