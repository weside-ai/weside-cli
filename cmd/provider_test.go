package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
