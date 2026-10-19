package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsUnsafeArgumentsWithoutOutput(t *testing.T) {
	t.Setenv("MIGRATION_DATABASE_URL", "")
	for _, args := range [][]string{
		nil,
		{"--account", "account", "--apply"},
		{"--account", "account", "--expected-sha256", strings.Repeat("a", 64)},
		{"--unknown=synthetic-private-input"},
		{"--account", "account", "positional"},
		{"--account", "account"},
	} {
		var output bytes.Buffer
		err := run(args, &output)
		if err == nil || output.Len() != 0 || strings.Contains(err.Error(), "synthetic-private-input") {
			t.Fatal("invalid command accepted or leaked its input")
		}
	}
}
