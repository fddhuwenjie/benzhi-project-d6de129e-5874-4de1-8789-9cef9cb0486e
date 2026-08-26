package domain

// IsTerminal reports whether a case can no longer accept business changes.
func IsTerminal(status CaseStatus) bool { return status == Archived }

// IsHold reports whether the workflow is paused for discrepancy handling.
func IsHold(status CaseStatus) bool { return status == CustodyHold || status == IntakeHold }
