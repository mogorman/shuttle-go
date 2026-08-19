package main

import "testing"

func TestRuneKeyCode(t *testing.T) {
	cases := []struct {
		r     rune
		code  int
		shift bool
	}{
		{'a', 30, false},
		{'A', 30, true},
		{' ', 57, false},
		{'!', 11, true},
		{'@', 2, true},
		{'~', 41, true},
		{'0', 11, false},
		{'.', 52, false},
	}
	for _, c := range cases {
		code, shift, err := runeKeyCode(c.r)
		if err != nil {
			t.Errorf("runeKeyCode(%q) unexpected error: %v", c.r, err)
			continue
		}
		if code != c.code || shift != c.shift {
			t.Errorf("runeKeyCode(%q) = (%d, %v), want (%d, %v)", c.r, code, shift, c.code, c.shift)
		}
	}

	if _, _, err := runeKeyCode('€'); err == nil {
		t.Error("runeKeyCode(€) should error (unmapped rune)")
	}
}
