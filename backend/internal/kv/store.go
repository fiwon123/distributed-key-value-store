// Package kv implements the in-memory key-value state machine.
//
// The store is deliberately simple: a mutex-guarded map. All state changes are
// expected to flow through the Raft log (in later phases) before reaching it;
// this package only holds the applied state.
package kv

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Store is a concurrency-safe in-memory key-value map.
type Store struct {
	mu sync.RWMutex
	m  map[string]string
}

// New returns an empty Store ready for use.
func New() *Store {
	return &Store{m: make(map[string]string)}
}

// Get returns the value stored for key and whether the key exists.
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	return v, ok
}

// Put sets key to value, overwriting any previous value.
func (s *Store) Put(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

// Delete removes key and reports whether the key existed.
func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[key]
	delete(s.m, key)
	return ok
}

// Len returns the number of keys currently stored.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

// Keys returns every stored key in sorted order. The result is a fresh slice,
// so callers may keep or modify it freely.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Snapshot returns a JSON image of the store, suitable for a Raft snapshot and
// for Restore to rebuild. The encoding is deterministic (Go sorts map keys).
func (s *Store) Snapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.Marshal(s.m)
	if err != nil {
		return nil, fmt.Errorf("kv: encode snapshot: %w", err)
	}
	return data, nil
}

// Restore replaces the store's contents with a snapshot produced by Snapshot.
func (s *Store) Restore(data []byte) error {
	contents := map[string]string{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &contents); err != nil {
			return fmt.Errorf("kv: decode snapshot: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = contents
	return nil
}
