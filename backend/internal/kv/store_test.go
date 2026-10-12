package kv

import (
	"fmt"
	"sync"
	"testing"
)

func TestStorePutGet(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantOK  bool
		wantVal string
	}{
		{name: "present key", key: "alpha", value: "one", wantOK: true, wantVal: "one"},
		{name: "empty value", key: "empty", value: "", wantOK: true, wantVal: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.Put(tc.key, tc.value)
			got, ok := s.Get(tc.key)
			if ok != tc.wantOK {
				t.Fatalf("Get(%q) ok = %v, want %v", tc.key, ok, tc.wantOK)
			}
			if got != tc.wantVal {
				t.Fatalf("Get(%q) = %q, want %q", tc.key, got, tc.wantVal)
			}
		})
	}
}

func TestStoreGetMissing(t *testing.T) {
	s := New()
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get on empty store reported a key as present")
	}
}

func TestStorePutOverwrite(t *testing.T) {
	s := New()
	s.Put("k", "first")
	s.Put("k", "second")
	got, ok := s.Get("k")
	if !ok || got != "second" {
		t.Fatalf("Get after overwrite = %q, %v; want \"second\", true", got, ok)
	}
	if n := s.Len(); n != 1 {
		t.Fatalf("Len after overwrite = %d, want 1", n)
	}
}

func TestStoreDelete(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		seed       bool
		wantDelete bool
		wantLen    int
	}{
		{name: "existing key", key: "k", seed: true, wantDelete: true, wantLen: 0},
		{name: "missing key", key: "k", seed: false, wantDelete: false, wantLen: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			if tc.seed {
				s.Put(tc.key, "v")
			}
			if got := s.Delete(tc.key); got != tc.wantDelete {
				t.Fatalf("Delete(%q) = %v, want %v", tc.key, got, tc.wantDelete)
			}
			if _, ok := s.Get(tc.key); ok {
				t.Fatalf("key %q still present after Delete", tc.key)
			}
			if got := s.Len(); got != tc.wantLen {
				t.Fatalf("Len = %d, want %d", got, tc.wantLen)
			}
		})
	}
}

func TestStoreKeys(t *testing.T) {
	s := New()
	s.Put("beta", "2")
	s.Put("alpha", "1")
	s.Put("gamma", "3")

	got := s.Keys()
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Keys() = %v, want %v", got, want)
		}
	}
}

func TestStoreKeysEmpty(t *testing.T) {
	if got := New().Keys(); len(got) != 0 {
		t.Fatalf("Keys() on empty store = %v, want empty", got)
	}
}

func TestStoreKeysSnapshotIndependent(t *testing.T) {
	s := New()
	s.Put("a", "1")

	keys := s.Keys()
	s.Put("b", "2")

	if len(keys) != 1 || keys[0] != "a" {
		t.Fatalf("Keys() result changed after a later Put: %v", keys)
	}
}

func TestStoreSnapshotRestoreRoundTrip(t *testing.T) {
	s := New()
	s.Put("a", "1")
	s.Put("b", "2")

	data, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	restored := New()
	if err := restored.Restore(data); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for _, key := range []string{"a", "b"} {
		want, _ := s.Get(key)
		got, ok := restored.Get(key)
		if !ok || got != want {
			t.Fatalf("restored.Get(%q) = %q, %v; want %q, true", key, got, ok, want)
		}
	}
	if n := restored.Len(); n != 2 {
		t.Fatalf("restored Len = %d, want 2", n)
	}
}

func TestStoreRestoreReplacesContents(t *testing.T) {
	s := New()
	s.Put("old", "value")

	data, err := New().Snapshot() // an empty snapshot
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if err := s.Restore(data); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if n := s.Len(); n != 0 {
		t.Fatalf("Len after restoring empty snapshot = %d, want 0", n)
	}
}

func TestStoreRestoreRejectsGarbage(t *testing.T) {
	if err := New().Restore([]byte("not json")); err == nil {
		t.Fatal("Restore of garbage data should fail")
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := New()
	const workers = 8
	const ops = 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("w%d-k%d", w, i%10)
				s.Put(key, "v")
				s.Get(key)
				s.Delete(key)
			}
		}(w)
	}
	wg.Wait()

	if n := s.Len(); n != 0 {
		t.Fatalf("Len after concurrent put/delete = %d, want 0", n)
	}
}
