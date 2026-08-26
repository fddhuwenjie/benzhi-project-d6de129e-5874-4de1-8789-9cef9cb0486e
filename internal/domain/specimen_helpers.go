package domain

// ActiveSpecimens returns specimens that were not explicitly retracted.
func ActiveSpecimens(c *ProvenanceCase) []SpecimenRecord {
	if c == nil {
		return nil
	}
	out := make([]SpecimenRecord, 0, len(c.Specimens))
	for _, s := range c.Specimens {
		if s.Status != "RETRACTED" {
			out = append(out, s)
		}
	}
	return out
}
