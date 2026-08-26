package domain

// IntakeCounts summarizes specimen intake outcomes for reporting.
func IntakeCounts(c *ProvenanceCase) (passed, failed, pending uint32) {
	if c == nil {
		return
	}
	for _, s := range c.Specimens {
		switch s.IntakeResult {
		case IntakePassed:
			passed++
		case IntakeFailed:
			failed++
		default:
			pending++
		}
	}
	return
}
