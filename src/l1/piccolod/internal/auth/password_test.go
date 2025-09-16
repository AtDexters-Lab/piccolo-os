package auth

import "testing"

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		password string
		ok       bool
	}{
		{password: "short", ok: false},
		{password: "alllowercase123", ok: false},
		{password: "NOLOWERCASE123!", ok: true},
		{password: "NoDigits!!!!", ok: true},
		{password: "ValidPass123!", ok: true},
		{password: "EmojiStrong💪Pass1", ok: true},
		{password: "Has space 123!", ok: false},
	}
	for _, tc := range tests {
		err := ValidatePasswordStrength(tc.password)
		if tc.ok && err != nil {
			t.Errorf("expected %q to be accepted: %v", tc.password, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("expected %q to be rejected", tc.password)
		}
	}
}
