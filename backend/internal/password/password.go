// Package password centralizes the password rule so every
// password-setting handler (change, force-reset, create-user, setup
// wizard) checks the exact same thing. A single length comparison used
// to be simple enough to repeat inline at each call site; a multi-
// condition rule is exactly the kind of thing that's easy to let drift
// if repeated by hand.
package password

import "errors"

var ErrInvalid = errors.New("password_invalid")

// Validate enforces: 8-16 characters, at least one uppercase letter, at
// least one digit. Symbols are neither required nor restricted.
func Validate(pw string) error {
	if len(pw) < 8 || len(pw) > 16 {
		return ErrInvalid
	}
	var hasUpper, hasDigit bool
	for _, r := range pw {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasDigit {
		return ErrInvalid
	}
	return nil
}
