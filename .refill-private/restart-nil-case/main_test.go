package restartnilcase_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"fossil-provenance-ledger/internal/store"
)

func TestMalformedSnapshotDoesNotCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	payload := map[string]any{
		"Cases":  map[string]any{"broken-case": nil},
		"Events": map[string]any{},
		"Idem":   map[string]any{},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("malformed snapshot caused startup panic: %v", r)
		}
	}()
	s := store.New(path)
	if healthy, loadErr := s.Healthy(); healthy || loadErr == nil {
		t.Fatalf("malformed snapshot should be reported unhealthy, healthy=%v err=%v", healthy, loadErr)
	}
}
