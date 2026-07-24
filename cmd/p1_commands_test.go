package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weside-ai/weside-cli/internal/api"
)

func p1Server(t *testing.T, match func(method, path string) (status int, body string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := match(r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
		}
		_, _ = io.WriteString(w, body)
	}))
}

func TestSkillsListCommand(t *testing.T) {
	srv := p1Server(t, func(method, path string) (int, string) {
		if method == http.MethodGet && strings.HasSuffix(path, "/companions/1/skills") {
			return 0, `{"skills":[{"skill_definition_id":7,"enabled":true,"skill":{"id":7,"name":"echo","version":"1.0"}}],"total":1}`
		}
		return http.StatusNotFound, ""
	})
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	var result map[string]any
	if err := client.Get(context.Background(), "/companions/1/skills", &result); err != nil {
		t.Fatalf("get: %v", err)
	}
	skills, _ := result["skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
}

func TestTriggersListCommand(t *testing.T) {
	srv := p1Server(t, func(method, path string) (int, string) {
		if method == http.MethodGet && strings.HasSuffix(path, "/companions/2/triggers") {
			return 0, `{"triggers":[{"id":"u-1","trigger_type":"interval","attention_tags":["morning"],"enabled":true,"next_trigger_at":null}],"total":1}`
		}
		return http.StatusNotFound, ""
	})
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	var result map[string]any
	if err := client.Get(context.Background(), "/companions/2/triggers", &result); err != nil {
		t.Fatalf("get: %v", err)
	}
	triggers, _ := result["triggers"].([]any)
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if got := joinAnySlice(triggers[0].(map[string]any)["attention_tags"]); got != "morning" {
		t.Errorf("tags = %q want morning", got)
	}
}

func TestMemoriesGetCommand(t *testing.T) {
	srv := p1Server(t, func(method, path string) (int, string) {
		if method == http.MethodGet && strings.HasSuffix(path, "/companions/3/memories/42") {
			return 0, `{"id":42,"type":"fact","title":"t","content":"body","memory_group_id":42,"version":1,"tags":["x"]}`
		}
		return http.StatusNotFound, ""
	})
	defer srv.Close()

	client := api.NewClient(srv.URL, "token")
	var result map[string]any
	if err := client.Get(context.Background(), "/companions/3/memories/42", &result); err != nil {
		t.Fatalf("get: %v", err)
	}
	if fmt.Sprintf("%v", result["title"]) != "t" {
		t.Errorf("title = %v want t", result["title"])
	}
	if got := joinAnySlice(result["tags"]); got != "x" {
		t.Errorf("tags = %q want x", got)
	}
}

func TestTriggersToggleRequiresEnabledFlag(t *testing.T) {
	// Without --enabled the command must error before any request.
	if err := triggersToggleCmd.RunE(triggersToggleCmd, []string{"1", "u-1"}); err == nil {
		t.Error("expected --enabled gate to block toggle")
	}
}
