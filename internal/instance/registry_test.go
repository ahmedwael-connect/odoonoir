package instance

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRegistryConcurrentPuts(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRegistry(filepath.Join(dir, "instances.json"))
	if err != nil {
		t.Fatal(err)
	}
	const n = 24
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			inst := &Instance{
				Name:    fmt.Sprintf("inst_%02d", i),
				Version: "18.0",
				Port:    8100 + i,
				DBName:  "db_" + fmt.Sprint(i),
			}
			if err := r.Put(inst); err != nil {
				t.Errorf("put: %v", err)
			}
		}(i)
	}
	wg.Wait()
	all, err := r.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Fatalf("expected %d instances after concurrent writes, got %d", n, len(all))
	}
}

func TestRegistryConcurrentReadModifyWrite(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRegistry(filepath.Join(dir, "instances.json"))
	if err != nil {
		t.Fatal(err)
	}
	base := &Instance{Name: "myapp", Version: "18.0", Port: 8069, DBName: "myapp"}
	if err := r.Put(base); err != nil {
		t.Fatal(err)
	}
	// read-modify-write cycles must not lose each other's changes
	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			inst, err := r.Get("myapp")
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			time.Sleep(1 * time.Millisecond)
			inst.Description = fmt.Sprintf("write %d", i)
			if err := r.Put(inst); err != nil {
				t.Errorf("put: %v", err)
			}
		}(i)
	}
	wg.Wait()
	// no corruption: still exactly one myapp entry
	all, err := r.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Name != "myapp" {
		t.Fatalf("registry corrupted: %v", all)
	}
}
