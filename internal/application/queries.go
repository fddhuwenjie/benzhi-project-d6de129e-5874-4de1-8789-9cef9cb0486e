package application

import "fossil-provenance-ledger/internal/domain"

// SummarizeCase provides the compact query model used by operational probes.
func (a *App) SummarizeCase(id string) (domain.CaseSummary, error) {
	c, err := a.Store.Get(id)
	if err != nil {
		return domain.CaseSummary{}, err
	}
	return domain.Summarize(c), nil
}
