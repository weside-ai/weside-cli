package cmd

import (
	"bytes"
	"testing"
)

func TestCSVSlice(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := csvSlice(tt.in)
		if !equalStrs(got, tt.want) {
			t.Errorf("csvSlice(%q) = %v want %v", tt.in, got, tt.want)
		}
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestJoinAnySlice(t *testing.T) {
	if got := joinAnySlice([]any{"a", "b"}); got != "a,b" {
		t.Errorf("got %q want a,b", got)
	}
	if got := joinAnySlice(nil); got != "-" {
		t.Errorf("got %q want -", got)
	}
}

func TestParseJSONConfig(t *testing.T) {
	cfg, ok := parseJSONConfig(`{"k":"v"}`)
	if !ok || cfg["k"] != "v" {
		t.Errorf("unexpected: %v ok=%v", cfg, ok)
	}
	if _, ok := parseJSONConfig(""); ok {
		t.Error("empty should be not ok")
	}
	if _, ok := parseJSONConfig("not json"); ok {
		t.Error("invalid json should be not ok")
	}
}

func TestReadBodyFromFileOrStdin(t *testing.T) {
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	stdinReader = bytes.NewBufferString("hello stdin")
	got, err := readBodyFromFileOrStdin("-")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "hello stdin" {
		t.Errorf("got %q", got)
	}
}
