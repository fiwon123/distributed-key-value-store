package main

import (
	"testing"

	"github.com/fiwon123/distributed-key-value-store/internal/kv"
)

func TestParsePeers(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty", in: "", want: map[string]string{}},
		{name: "blank", in: "   ", want: map[string]string{}},
		{
			name: "single",
			in:   "node2=http://localhost:8082",
			want: map[string]string{"node2": "http://localhost:8082"},
		},
		{
			name: "multiple with whitespace",
			in:   " node2=http://localhost:8082 , node3=http://localhost:8083 ",
			want: map[string]string{
				"node2": "http://localhost:8082",
				"node3": "http://localhost:8083",
			},
		},
		{name: "missing url", in: "node2=", wantErr: true},
		{name: "missing separator", in: "node2", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePeers(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePeers(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePeers(%q) unexpected error: %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parsePeers(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("parsePeers(%q)[%q] = %q, want %q", tc.in, k, got[k], v)
				}
			}
		})
	}
}

func TestDirectBackend(t *testing.T) {
	b := directBackend{store: kv.New()}
	if _, ok := b.Get("k"); ok {
		t.Fatal("Get on empty backend reported present")
	}
	if err := b.Put("k", "v"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, ok := b.Get("k"); !ok || got != "v" {
		t.Fatalf("Get = %q, %v; want v, true", got, ok)
	}
	deleted, err := b.Delete("k")
	if err != nil || !deleted {
		t.Fatalf("Delete = %v, %v; want true, nil", deleted, err)
	}
	if _, ok := b.Get("k"); ok {
		t.Fatal("key still present after delete")
	}
}
