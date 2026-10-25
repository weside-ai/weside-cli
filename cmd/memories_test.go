package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestMemoriesListPath(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"no flags, no filter", nil, "/companions/7/memories", false},
		{"autoload true", []string{"--autoload"}, "/companions/7/memories?autoload=true", false},
		{"autoload false is a filter, not unset", []string{"--autoload=false"}, "/companions/7/memories?autoload=false", false},
		{
			"paging and type",
			[]string{"--page", "3", "--limit", "50", "--type", "fact", "--autoload=true"},
			"/companions/7/memories?autoload=true&limit=50&memory_type=fact&page=3",
			false,
		},
		{"limit above the API cap", []string{"--limit", "101"}, "", true},
		{"page zero", []string{"--page", "0"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cobra.Command{Use: "list"}
			addMemoriesListFilterFlags(c)
			if err := c.ParseFlags(tt.args); err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, err := memoriesListPath(c, "7")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMemoryListRowShowsIDsAndPin(t *testing.T) {
	m := map[string]any{
		"id": float64(12), "memory_id": float64(5), "autoload": true,
		"type": "fact", "title": "Name", "content": "Foxy",
	}
	want := []string{"12", "5", "yes", "fact", "Name", "Foxy"}
	if got := memoryListRow(m); !reflect.DeepEqual(got, want) {
		t.Errorf("row = %v, want %v", got, want)
	}
	m["autoload"] = false
	if got := memoryListRow(m)[2]; got != "no" {
		t.Errorf("pinned = %q, want no", got)
	}
}
