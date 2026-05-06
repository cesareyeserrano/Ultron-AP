package database

import (
	"errors"
	"testing"
)

// @aitri-trace BG-029 BL-004
func TestIsDuplicateColumnErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sqlite duplicate column", errors.New("SQL logic error: duplicate column name: source (1)"), true},
		{"sqlite duplicate column lowercase", errors.New("duplicate column name: source"), true},
		{"unrelated error", errors.New("database is locked"), false},
		{"missing table", errors.New("no such table: ActionLog"), false},
		{"empty message", errors.New(""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDuplicateColumnErr(tc.err); got != tc.want {
				t.Fatalf("isDuplicateColumnErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
