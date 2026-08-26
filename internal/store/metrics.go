package store

// Stats exposes inexpensive in-memory store counters for health and diagnostics.
type Stats struct {
	Cases int `json:"cases"`
}

func (s *Store) Stats() Stats { return Stats{Cases: len(s.List())} }
