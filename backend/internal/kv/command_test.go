package kv

import "testing"

func TestCommandRoundTrip(t *testing.T) {
	cases := []Command{
		{Op: OpPut, Key: "k", Value: "v"},
		{Op: OpDelete, Key: "k"},
	}
	for _, want := range cases {
		data, err := want.Encode()
		if err != nil {
			t.Fatalf("Encode(%+v): %v", want, err)
		}
		got, err := DecodeCommand(data)
		if err != nil {
			t.Fatalf("DecodeCommand(%+v): %v", want, err)
		}
		if got != want {
			t.Fatalf("round trip = %+v, want %+v", got, want)
		}
	}
}

func TestDecodeCommandInvalid(t *testing.T) {
	if _, err := DecodeCommand([]byte("{not json")); err == nil {
		t.Fatal("DecodeCommand(invalid) = nil error, want non-nil")
	}
}

func TestApplyUpdatesStore(t *testing.T) {
	s := New()

	if err := s.Apply(Command{Op: OpPut, Key: "k", Value: "v"}); err != nil {
		t.Fatalf("apply put: %v", err)
	}
	if got, ok := s.Get("k"); !ok || got != "v" {
		t.Fatalf("Get = %q, %v; want v, true", got, ok)
	}
	if err := s.Apply(Command{Op: OpDelete, Key: "k"}); err != nil {
		t.Fatalf("apply delete: %v", err)
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("key still present after apply delete")
	}
}

func TestApplyUnknownOp(t *testing.T) {
	s := New()
	if err := s.Apply(Command{Op: "nope", Key: "k"}); err == nil {
		t.Fatal("Apply(unknown op) = nil error, want non-nil")
	}
}
