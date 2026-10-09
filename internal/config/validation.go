// Package config provides business-specific validation utilities.
package config

import "regexp"

// BusinessValidation provides validation for business-specific rules.
type BusinessValidation struct {
	usernameRegex *regexp.Regexp
}

// NewBusinessValidation creates a new business validation instance.
func NewBusinessValidation() *BusinessValidation {
	return &BusinessValidation{
		// 5-20 characters: letters, digits and underscores, starting with a letter.
		usernameRegex: regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,19}$`),
	}
}

// IsUsername validates if a string matches the username format.
func (v *BusinessValidation) IsUsername(s string) bool {
	return v.usernameRegex.MatchString(s)
}

// BusinessValidator is the global instance for business validation.
var BusinessValidator = NewBusinessValidation()
