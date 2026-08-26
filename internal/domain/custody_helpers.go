package domain

// OpenCustodyDiscrepancies counts unresolved transfer discrepancies.
func OpenCustodyDiscrepancies(c *ProvenanceCase) int {
	if c == nil {
		return 0
	}
	n := 0
	for _, t := range c.Transfers {
		for _, d := range t.Discrepancies {
			if d.Status != "RESOLVED" {
				n++
			}
		}
	}
	return n
}
