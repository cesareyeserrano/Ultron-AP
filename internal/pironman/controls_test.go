package pironman

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBoolOrString(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "json true", raw: json.RawMessage("true"), want: true},
		{name: "json false", raw: json.RawMessage("false"), want: false},
		{name: "string on", raw: json.RawMessage(`"on"`), want: true},
		{name: "string off", raw: json.RawMessage(`"off"`), want: false},
		{name: "string 1", raw: json.RawMessage(`"1"`), want: true},
		{name: "string true", raw: json.RawMessage(`"true"`), want: true},
		{name: "invalid value", raw: json.RawMessage(`"nope"`), want: false},
		{name: "malformed", raw: json.RawMessage("{"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseBoolOrString(tt.raw))
		})
	}
}
