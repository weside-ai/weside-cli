package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/weside-ai/weside-cli/internal/auth"
)

// fakeSupabase serves /auth/v1/user, /challenge and /verify. It accepts the
// code goodCode and counts challenges and verifies.
type fakeSupabase struct {
	factorsJSON string
	goodCode    string
	challenges  atomic.Int32
	verifies    atomic.Int32
}

func (f *fakeSupabase) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "anon" || r.Header.Get("Authorization") != "Bearer aal1-access" {
			t.Errorf("%s %s: wrong apikey/authorization headers", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/auth/v1/user":
			_, _ = w.Write([]byte(`{"id":"u1","factors":` + f.factorsJSON + `}`))
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/factors/f-totp/challenge":
			n := f.challenges.Add(1)
			_, _ = w.Write([]byte(`{"id":"ch-` + string(rune('0'+n)) + `","type":"totp"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/factors/f-totp/verify":
			f.verifies.Add(1)
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			want := "ch-" + string(rune('0'+f.challenges.Load()))
			if body["challenge_id"] != want {
				t.Errorf("verify challenge_id = %q, want the latest %q", body["challenge_id"], want)
			}
			if body["code"] != f.goodCode {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error_code":"mfa_verification_failed"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"aal2-access","refresh_token":"aal2-refresh","expires_in":3600,"token_type":"bearer"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

const verifiedTOTP = `[{"id":"f-unverified","factor_type":"totp","status":"unverified"},{"id":"f-pk","factor_type":"webauthn","status":"verified"},{"id":"f-totp","factor_type":"totp","status":"verified"}]`

var loginTokens = &auth.PKCEResult{AccessToken: "aal1-access", RefreshToken: "aal1-refresh"}

func TestCompleteMFA_VerifiedFactorUpgradesSession(t *testing.T) {
	f := &fakeSupabase{factorsJSON: verifiedTOTP, goodCode: "123456"}
	srv := f.server(t)
	defer srv.Close()

	prompts := 0
	res, err := auth.CompleteMFA(srv.URL, "anon", loginTokens, func(int) (string, error) {
		prompts++
		return " 123456\n", nil
	})
	if err != nil {
		t.Fatalf("CompleteMFA: %v", err)
	}
	if res.AccessToken != "aal2-access" || res.RefreshToken != "aal2-refresh" {
		t.Errorf("got %q/%q, want the verify tokens", res.AccessToken, res.RefreshToken)
	}
	if prompts != 1 || f.challenges.Load() != 1 || f.verifies.Load() != 1 {
		t.Errorf("prompts=%d challenges=%d verifies=%d, want 1/1/1", prompts, f.challenges.Load(), f.verifies.Load())
	}
}

func TestCompleteMFA_NoVerifiedFactorNeverPrompts(t *testing.T) {
	for name, factors := range map[string]string{
		"null":                  `null`,
		"empty":                 `[]`,
		"unverified totp":       `[{"id":"f-totp","factor_type":"totp","status":"unverified"}]`,
		"verified passkey only": `[{"id":"f-pk","factor_type":"webauthn","status":"verified"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeSupabase{factorsJSON: factors}
			srv := f.server(t)
			defer srv.Close()
			res, err := auth.CompleteMFA(srv.URL, "anon", loginTokens, func(int) (string, error) {
				t.Fatal("prompt called without a verified TOTP factor")
				return "", nil
			})
			if err != nil {
				t.Fatalf("CompleteMFA: %v", err)
			}
			if res != loginTokens {
				t.Errorf("tokens changed without a factor: %+v", res)
			}
			if f.challenges.Load() != 0 {
				t.Errorf("challenges = %d, want 0", f.challenges.Load())
			}
		})
	}
}

func TestCompleteMFA_OneRetryThenSuccess(t *testing.T) {
	f := &fakeSupabase{factorsJSON: verifiedTOTP, goodCode: "123456"}
	srv := f.server(t)
	defer srv.Close()
	codes := []string{"000000", "123456"}
	var attempts []int
	res, err := auth.CompleteMFA(srv.URL, "anon", loginTokens, func(attempt int) (string, error) {
		attempts = append(attempts, attempt)
		return codes[attempt-1], nil
	})
	if err != nil {
		t.Fatalf("CompleteMFA: %v", err)
	}
	if res.AccessToken != "aal2-access" {
		t.Errorf("access token = %q, want aal2-access", res.AccessToken)
	}
	if len(attempts) != 2 || attempts[1] != 2 || f.challenges.Load() != 2 {
		t.Errorf("attempts=%v challenges=%d, want [1 2] and a fresh challenge per attempt", attempts, f.challenges.Load())
	}
}

func TestCompleteMFA_WrongCodeTwiceFails(t *testing.T) {
	f := &fakeSupabase{factorsJSON: verifiedTOTP, goodCode: "123456"}
	srv := f.server(t)
	defer srv.Close()
	prompts := 0
	res, err := auth.CompleteMFA(srv.URL, "anon", loginTokens, func(int) (string, error) {
		prompts++
		return "000000", nil
	})
	if err == nil {
		t.Fatalf("CompleteMFA returned %+v, want an error", res)
	}
	if !strings.Contains(err.Error(), "wrong 2 times") || !strings.Contains(err.Error(), "weside auth login") {
		t.Errorf("error = %q, want the wrong-twice message", err)
	}
	if prompts != 2 || f.verifies.Load() != 2 {
		t.Errorf("prompts=%d verifies=%d, want 2/2", prompts, f.verifies.Load())
	}
}

func TestCompleteMFA_ServerErrorOnVerifyDoesNotRetry(t *testing.T) {
	var challenges atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = w.Write([]byte(`{"factors":` + verifiedTOTP + `}`))
		case strings.HasSuffix(r.URL.Path, "/challenge"):
			challenges.Add(1)
			_, _ = w.Write([]byte(`{"id":"ch-1"}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	prompts := 0
	_, err := auth.CompleteMFA(srv.URL, "anon", loginTokens, func(int) (string, error) {
		prompts++
		return "123456", nil
	})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want the 500 verify failure", err)
	}
	if prompts != 1 {
		t.Errorf("prompts = %d, want 1 (no retry on a server error)", prompts)
	}
}
