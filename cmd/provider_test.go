package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestBuildSetRequestBody(t *testing.T) {
	groups := []any{
		map[string]any{
			"region": "EUR",
			"presets": []any{
				map[string]any{"id": float64(11)},
			},
		},
		map[string]any{
			"region": "USA",
			"presets": []any{
				map[string]any{"id": float64(22)},
			},
		},
		map[string]any{
			"region": "WESIDE",
			"presets": []any{
				map[string]any{"id": float64(33)},
			},
		},
	}

	tests := []struct {
		name       string
		presetID   int
		wantType   string
		wantRegion any
	}{
		{name: "EUR preset", presetID: 11, wantType: "region", wantRegion: "EUR"},
		{name: "USA preset", presetID: 22, wantType: "region", wantRegion: "USA"},
		{name: "WESIDE preset", presetID: 33, wantType: "weside", wantRegion: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := buildSetRequestBody(tt.presetID, groups)
			if err != nil {
				t.Fatalf("buildSetRequestBody: %v", err)
			}
			if got := body["type"]; got != tt.wantType {
				t.Errorf("type = %v, want %s", got, tt.wantType)
			}
			if got := body["region"]; got != tt.wantRegion {
				t.Errorf("region = %v, want %v", got, tt.wantRegion)
			}
			if got := body["preset_id"]; got != tt.presetID {
				t.Errorf("preset_id = %v, want %d", got, tt.presetID)
			}
		})
	}
}

func TestBuildSetRequestBodyRejectsUnknownPreset(t *testing.T) {
	_, err := buildSetRequestBody(99, []any{})
	if err == nil {
		t.Fatal("expected unknown preset error")
	}
	want := "preset_id 99 not found (use 'weside provider presets' to see valid IDs)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestProviderSetCommandUsesPresetRegion(t *testing.T) {
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/data-residency/presets":
			_, _ = w.Write([]byte(`{"groups":[{"region":"EUR","presets":[{"id":11}]}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/data-residency/":
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("decode PUT body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("WESIDE_TOKEN", "token")
	viper.Set("api_url", srv.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })

	if err := providerSetCmd.RunE(providerSetCmd, []string{"11"}); err != nil {
		t.Fatalf("provider set: %v", err)
	}

	if got := putBody["type"]; got != "region" {
		t.Errorf("type = %v, want region", got)
	}
	if got := putBody["region"]; got != "EUR" {
		t.Errorf("region = %v, want EUR", got)
	}
	if got := putBody["preset_id"]; got != float64(11) {
		t.Errorf("preset_id = %v, want 11", got)
	}
	if _, ok := putBody["quality"]; ok {
		t.Errorf("quality should be omitted, got %v", putBody["quality"])
	}
}

// WA-2257: the switch is the fourth shape of the data-residency union. It
// modifies the current selection, so the body must NOT restate region,
// quality or preset id — a caller that did would silently re-pick a preset
// while only meaning to flip a switch.
func TestProviderEuOnlySendsOnlyTheSwitch(t *testing.T) {
	for _, tt := range []struct {
		arg  string
		want bool
	}{{arg: "on", want: true}, {arg: "off", want: false}} {
		t.Run(tt.arg, func(t *testing.T) {
			var putBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/api/v1/data-residency/" {
					http.Error(w, "unexpected request", http.StatusNotFound)
					return
				}
				if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
					t.Errorf("decode PUT body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"eu_only":true,"eu_only_disables":[{"key":"web_search"}]}`))
			}))
			defer srv.Close()

			t.Setenv("WESIDE_TOKEN", "token")
			viper.Set("api_url", srv.URL)
			t.Cleanup(func() { viper.Set("api_url", "") })

			if err := providerEuOnlyCmd.RunE(providerEuOnlyCmd, []string{tt.arg}); err != nil {
				t.Fatalf("provider eu-only %s: %v", tt.arg, err)
			}

			if got := putBody["type"]; got != "eu_only" {
				t.Errorf("type = %v, want eu_only", got)
			}
			if got := putBody["eu_only"]; got != tt.want {
				t.Errorf("eu_only = %v, want %v", got, tt.want)
			}
			for _, key := range []string{"region", "quality", "preset_id"} {
				if _, ok := putBody[key]; ok {
					t.Errorf("%s must not be restated, got %v", key, putBody[key])
				}
			}
		})
	}
}

// The 409 is the one refusal a user can act on, and the server's sentence
// already says what to do. Wrapping it in "setting EU only: …" buries the
// instruction behind the failure.
func TestProviderEuOnlyPassesTheConflictSentenceThrough(t *testing.T) {
	const detail = "EU only can only be switched on while your inference runs in the EU."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"` + detail + `"}`))
	}))
	defer srv.Close()

	t.Setenv("WESIDE_TOKEN", "token")
	viper.Set("api_url", srv.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })

	err := providerEuOnlyCmd.RunE(providerEuOnlyCmd, []string{"on"})
	if err == nil {
		t.Fatal("expected the 409 to surface as an error")
	}
	if err.Error() != detail {
		t.Errorf("error = %q, want exactly the server's sentence %q", err.Error(), detail)
	}
}

func TestProviderEuOnlyRejectsAnythingButOnOff(t *testing.T) {
	if err := providerEuOnlyCmd.RunE(providerEuOnlyCmd, []string{"true"}); err == nil {
		t.Fatal("expected 'true' to be rejected — the verb takes on|off")
	}
}

// An older backend has no `eu_only` field. Printing "EU only: false" there
// would be a claim about a setting that does not exist on that server.
func TestPrintEuOnlySaysNothingWhenTheServerDidNot(t *testing.T) {
	out := capturedProviderOutput(t, map[string]any{"type": "region"})
	if out != "" {
		t.Errorf("printed %q for a response without eu_only, want nothing", out)
	}
}

func TestPrintEuOnlyCountsWhatIsHidden(t *testing.T) {
	out := capturedProviderOutput(t, map[string]any{
		"eu_only":           true,
		"eu_only_disables":  []any{map[string]any{}, map[string]any{}},
		"eu_only_available": true,
	})
	if !strings.Contains(out, "on") || !strings.Contains(out, "2 sources hidden") {
		t.Errorf("printed %q, want the state and the count", out)
	}
}

func TestPrintEuOnlyNamesWhyItIsUnavailable(t *testing.T) {
	out := capturedProviderOutput(t, map[string]any{
		"eu_only":           false,
		"eu_only_available": false,
	})
	if !strings.Contains(out, "needs the EU region") {
		t.Errorf("printed %q, want the reason it cannot be turned on", out)
	}
}

func capturedProviderOutput(t *testing.T, result map[string]any) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	stdout := os.Stdout
	os.Stdout = w
	printEuOnly(result)
	_ = w.Close()
	os.Stdout = stdout

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read: %v", err)
	}
	return buf.String()
}

// Measured against staging rc.3: cobra printed fifteen lines of usage above
// the 409, burying the one sentence that tells the user what to do. Usage is
// for a malformed invocation, not for a server that answered clearly.
func TestProviderEuOnlyDoesNotPrintUsageOnAServerError(t *testing.T) {
	if !providerEuOnlyCmd.SilenceUsage {
		t.Error("SilenceUsage is off — a 409 would be buried under the usage block")
	}
}
