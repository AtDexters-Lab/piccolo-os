package auth

import (
	"errors"
	"unicode"
)

var (
	errPasswordTooShort   = errors.New("password must be at least 12 characters long")
	errPasswordCategories = errors.New("password must include characters from at least three categories: uppercase, lowercase, digits, symbols")
	errPasswordWhitespace = errors.New("password cannot contain whitespace")
)

// ValidatePasswordStrength enforces the minimum password policy for the admin account.
// Requirements:
//   - at least 12 characters long
//   - may not contain whitespace characters
//   - contains characters from at least three categories: uppercase, lowercase, digits, symbols
func ValidatePasswordStrength(password string) error {
	if len(password) < 12 {
		return errPasswordTooShort
	}
	var (
		hasUpper  bool
		hasLower  bool
		hasDigit  bool
		hasSymbol bool
	)
	for _, r := range password {
		switch {
		case unicode.IsSpace(r):
			return errPasswordWhitespace
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSymbol = true
		default:
			// Treat any other printable character (e.g., emoji) as a symbol category.
			if !unicode.IsControl(r) {
				hasSymbol = true
			}
		}
	}
	categories := 0
	if hasUpper {
		categories++
	}
	if hasLower {
		categories++
	}
	if hasDigit {
		categories++
	}
	if hasSymbol {
		categories++
	}
	if categories < 3 {
		return errPasswordCategories
	}
	return nil
}
