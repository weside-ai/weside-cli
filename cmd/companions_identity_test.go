package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// WA-2674 AC10: `companions select A` then `companions identity` returned B,
// because identity followed the server-side selection instead of the local one.

// mcpPinRecorder serves a minimal MCP tools/call answer and records the
// X-Weside-Companion header of every request.
func mcpPinRecorder(t *testing.T) *[]string {
	t.Helper()
	var pins []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/mcp") {
			http.NotFound(w, r)
			return
		}
		pins = append(pins, r.Header.Get("X-Weside-Companion"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WESIDE_TOKEN", "test-token")
	viper.Set("api_url", srv.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })
	return &pins
}

func withDefaultCompanion(t *testing.T, name string) {
	t.Helper()
	viper.Set("default_companion", name)
	t.Cleanup(func() { viper.Set("default_companion", "") })
}

func TestIdentityPinsTheLocallySelectedCompanion(t *testing.T) {
	pins := mcpPinRecorder(t)
	withDefaultCompanion(t, "Nox")

	if err := companionsIdentityCmd.RunE(companionsIdentityCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if len(*pins) != 1 || (*pins)[0] != "Nox" {
		t.Fatalf("X-Weside-Companion = %q, want [\"Nox\"] (the locally selected companion)", *pins)
	}
}

func TestIdentityWithoutLocalSelectionSendsNoPin(t *testing.T) {
	pins := mcpPinRecorder(t)
	withDefaultCompanion(t, "")

	if err := companionsIdentityCmd.RunE(companionsIdentityCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if len(*pins) != 1 || (*pins)[0] != "" {
		t.Fatalf("X-Weside-Companion = %q, want no pin", *pins)
	}
}

func TestIdentityCompanionFlagWinsOverLocalSelection(t *testing.T) {
	pins := mcpPinRecorder(t)
	withDefaultCompanion(t, "Nox")
	identityCompanion = "Elise"
	t.Cleanup(func() { identityCompanion = "" })

	if err := companionsIdentityCmd.RunE(companionsIdentityCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if len(*pins) != 1 || (*pins)[0] != "Elise" {
		t.Fatalf("X-Weside-Companion = %q, want [\"Elise\"] (--companion wins)", *pins)
	}
}

// The MCP write verbs follow the same selection as identity.
func TestMCPSaveVerbsPinTheLocallySelectedCompanion(t *testing.T) {
	pins := mcpPinRecorder(t)
	withDefaultCompanion(t, "Nox")
	memorySaveContent = "c"
	goalSaveContent = "c"
	t.Cleanup(func() { memorySaveContent, goalSaveContent = "", "" })

	if err := memoriesSaveCmd.RunE(memoriesSaveCmd, []string{"m"}); err != nil {
		t.Fatalf("memories save: %v", err)
	}
	if err := goalsSaveCmd.RunE(goalsSaveCmd, []string{"g"}); err != nil {
		t.Fatalf("goals save: %v", err)
	}
	if len(*pins) != 2 || (*pins)[0] != "Nox" || (*pins)[1] != "Nox" {
		t.Fatalf("X-Weside-Companion = %q, want [\"Nox\" \"Nox\"]", *pins)
	}
}
