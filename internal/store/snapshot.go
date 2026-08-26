package store

import "encoding/json"

// Snapshot returns a detached case suitable for read-only inspection.
func (s *Store) Snapshot(id string) ([]byte, error) {
	c, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return json.Marshal(c)
}
