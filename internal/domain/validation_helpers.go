package domain

// ValidateRequiredText centralizes the non-empty text rule used by adapters.
func ValidateRequiredText(name, value string) error {
	if Normalize(value) == "" {
		return validation(name + " required")
	}
	return nil
}
