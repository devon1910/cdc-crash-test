package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestConfirmRequiresExactYES(t *testing.T) {
	for _, input := range []struct {
		response string
		want     bool
	}{
		{response: "YES\n", want: true},
		{response: "yes\n", want: false},
		{response: "NO\n", want: false},
	} {
		if got := confirm(strings.NewReader(input.response), &strings.Builder{}); got != input.want {
			t.Errorf("confirm(%q) = %t, want %t", input.response, got, input.want)
		}
	}
}

func TestIsDiskFull(t *testing.T) {
	if !isDiskFull(&pgconn.PgError{Code: "53100", Message: "disk full"}) {
		t.Fatal("SQLSTATE 53100 should be recognized as disk full")
	}
	if !isDiskFull(errors.New("could not extend file: No space left on device")) {
		t.Fatal("no-space operating-system error should be recognized")
	}
	if isDiskFull(&pgconn.PgError{Code: "23505", Message: "duplicate key"}) {
		t.Fatal("unrelated SQLSTATE should not be recognized as disk full")
	}
}
