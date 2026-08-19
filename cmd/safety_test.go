package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestSafetyReportFlagValidation(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		reasonCode string
		text       string
		wantErr    string
	}{
		{
			name:       "bad reason code names allowed values",
			userID:     "user-1",
			reasonCode: "not_a_reason",
			wantErr:    "harassment, unwanted_sexual, impersonation, spam_scam, other",
		},
		{
			name:       "other requires text",
			userID:     "user-1",
			reasonCode: "other",
			wantErr:    "--text is required when --reason-code is other",
		},
		{
			name:       "valid call",
			userID:     "user-1",
			reasonCode: "harassment",
			text:       "Please review this message.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSafetyReport(tt.userID, tt.reasonCode, tt.text)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("valid flags rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestSafetyVerbsAreRegistered(t *testing.T) {
	for _, name := range []string{"blocked", "report"} {
		cmd, _, err := safetyCmd.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("safety %s command missing: cmd=%v err=%v", name, cmd, err)
		}
	}
}

func TestSafetyReportRequestBodyAndJSONOutput(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/reports" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"report-1","status":"received"}`)
	}))
	defer srv.Close()

	restoreSafetyTestConfig(t, srv.URL, true)
	safetyReportUser = "user-7"
	safetyReportReasonCode = "harassment"
	safetyReportText = "They sent another threatening message."
	out := captureSafetyStdout(t, func() error {
		return safetyReportCmd.RunE(safetyReportCmd, nil)
	})

	wantBody := map[string]any{
		"target_type":    "user",
		"target_user_id": "user-7",
		"reason_code":    "harassment",
		"reason":         "They sent another threatening message.",
	}
	if fmt.Sprint(gotBody) != fmt.Sprint(wantBody) {
		t.Fatalf("request body = %v, want %v", gotBody, wantBody)
	}
	var gotOutput map[string]any
	if err := json.Unmarshal([]byte(out), &gotOutput); err != nil {
		t.Fatalf("report --json output is not JSON: %v; output=%q", err, out)
	}
	if gotOutput["id"] != "report-1" || gotOutput["status"] != "received" {
		t.Fatalf("report --json output = %v", gotOutput)
	}
}

func TestSafetyBlockedJSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/users/blocked" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":"user-2","display_name":"Morgan"}]`)
	}))
	defer srv.Close()

	restoreSafetyTestConfig(t, srv.URL, true)
	out := captureSafetyStdout(t, func() error {
		return safetyBlockedCmd.RunE(safetyBlockedCmd, nil)
	})
	var got []blockedUser
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("blocked --json output is not JSON: %v; output=%q", err, out)
	}
	if len(got) != 1 || got[0].ID != "user-2" || got[0].DisplayName != "Morgan" {
		t.Fatalf("blocked --json output = %+v", got)
	}
}

func restoreSafetyTestConfig(t *testing.T, serverURL string, jsonOutput bool) {
	t.Helper()
	oldAPIURL := viper.Get("api_url")
	oldJSON := viper.Get("json")
	oldUser, oldCode, oldText := safetyReportUser, safetyReportReasonCode, safetyReportText
	t.Setenv("WESIDE_TOKEN", "test-token")
	viper.Set("api_url", serverURL)
	viper.Set("json", jsonOutput)
	t.Cleanup(func() {
		viper.Set("api_url", oldAPIURL)
		viper.Set("json", oldJSON)
		safetyReportUser, safetyReportReasonCode, safetyReportText = oldUser, oldCode, oldText
	})
}

func captureSafetyStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if runErr != nil {
		t.Fatalf("command error: %v", runErr)
	}
	if readErr != nil {
		t.Fatalf("reading output: %v", readErr)
	}
	return string(out)
}
