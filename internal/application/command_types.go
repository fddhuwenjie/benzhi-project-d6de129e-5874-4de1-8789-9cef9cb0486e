package application

// RevisionExpectation is shared by write command adapters.
type RevisionExpectation struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ActorID          string `json:"actor_id"`
}
