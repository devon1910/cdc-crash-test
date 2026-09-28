package main

import (
	"strings"
	"testing"
)

func TestConfirmRequiresExactYES(t *testing.T) {
	for _, input := range []struct {
		response string
		want     bool
	}{
		{response: "YES\n", want: true},
		{response: "yes\n", want: false},
		{response: "no\n", want: false},
	} {
		if got := confirm(strings.NewReader(input.response), &strings.Builder{}); got != input.want {
			t.Errorf("confirm(%q) = %t, want %t", input.response, got, input.want)
		}
	}
}
