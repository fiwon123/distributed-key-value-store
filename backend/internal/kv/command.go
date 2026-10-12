package kv

import (
	"encoding/json"
	"fmt"
)

// Op identifies a state-machine operation.
type Op string

const (
	// OpPut sets a key to a value.
	OpPut Op = "put"
	// OpDelete removes a key.
	OpDelete Op = "delete"
)

// Command is the state-machine operation carried in a Raft log entry. It is
// encoded to JSON before being proposed, and decoded when a committed entry is
// applied.
type Command struct {
	Op    Op     `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// Encode serialises the command for the Raft log.
func (c Command) Encode() ([]byte, error) {
	return json.Marshal(c)
}

// DecodeCommand parses a command from a committed log entry.
func DecodeCommand(data []byte) (Command, error) {
	var c Command
	if err := json.Unmarshal(data, &c); err != nil {
		return Command{}, fmt.Errorf("kv: decode command: %w", err)
	}
	return c, nil
}

// Apply executes a command against the store. It is the function the Raft node
// calls for each committed entry; it must be deterministic and idempotent
// (restart can replay entries).
func (s *Store) Apply(c Command) error {
	switch c.Op {
	case OpPut:
		s.Put(c.Key, c.Value)
		return nil
	case OpDelete:
		s.Delete(c.Key)
		return nil
	default:
		return fmt.Errorf("kv: unknown op %q", c.Op)
	}
}
