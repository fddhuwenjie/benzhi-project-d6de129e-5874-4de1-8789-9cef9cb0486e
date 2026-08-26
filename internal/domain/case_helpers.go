package domain

// CaseSummary is a compact stable representation for logs and health probes.
type CaseSummary struct {
	ID       string     `json:"id"`
	Status   CaseStatus `json:"status"`
	Revision uint64     `json:"revision"`
}

func Summarize(c *ProvenanceCase) CaseSummary {
	if c == nil {
		return CaseSummary{}
	}
	return CaseSummary{ID: c.ID, Status: c.Status, Revision: c.Revision}
}
