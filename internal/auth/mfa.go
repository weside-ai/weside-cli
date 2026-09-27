package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrWrongCode means Supabase rejected the TOTP code itself (400/422 on verify),
// as opposed to a network or server failure. Only this error earns a retry.
var ErrWrongCode = errors.New("the code was not accepted")

// CodePrompt asks the user for a 6-digit code. attempt starts at 1.
type CodePrompt func(attempt int) (string, error)

// maxCodeAttempts is one retry after a wrong code, then a clear error.
const maxCodeAttempts = 2

// VerifiedTOTPFactor returns the id of the user's first verified TOTP factor,
// or "" when the user has none. It reads GET /auth/v1/user with the session's
// own access token.
//
// Only TOTP counts, on purpose: the weside backend's policy counts exactly the
// same thing (weside-core `current_user_has_verified_mfa_factor()`,
// factor_type 'totp'), and phone/WebAuthn MFA are switched off in Supabase. A
// user with only another factor type is therefore no factor holder to the
// backend and gets no 403 — no login loop.
func VerifiedTOTPFactor(supabaseURL, supabaseAnonKey, accessToken string) (string, error) {
	var user struct {
		Factors []struct {
			ID         string `json:"id"`
			FactorType string `json:"factor_type"`
			Status     string `json:"status"`
		} `json:"factors"`
	}
	status, errCode, err := supabaseJSON(http.MethodGet, supabaseURL, "/auth/v1/user", supabaseAnonKey, accessToken, nil, &user)
	if err != nil {
		return "", fmt.Errorf("reading two-factor status: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("reading two-factor status failed (%d%s) — try `weside auth login` again", status, errCode)
	}
	for _, f := range user.Factors {
		if f.FactorType == "totp" && f.Status == "verified" {
			return f.ID, nil
		}
	}
	return "", nil
}

// VerifyTOTP runs one challenge + verify against the factor with the session's
// own access token. Supabase answers with tokens of the same session raised to
// aal2 (measured on staging, WA-2309 phase-0 receipt finding 8). A 400 or 422
// on verify (Supabase's answers to a malformed or wrong code) returns
// ErrWrongCode; any other failure is returned as is.
func VerifyTOTP(supabaseURL, supabaseAnonKey, accessToken, factorID, code string) (*PKCEResult, error) {
	base := "/auth/v1/factors/" + url.PathEscape(factorID)

	var challenge struct {
		ID string `json:"id"`
	}
	status, errCode, err := supabaseJSON(http.MethodPost, supabaseURL, base+"/challenge", supabaseAnonKey, accessToken, map[string]any{}, &challenge)
	if err != nil {
		return nil, fmt.Errorf("two-factor challenge: %w", err)
	}
	if status != http.StatusOK || challenge.ID == "" {
		return nil, fmt.Errorf("two-factor challenge failed (%d%s)", status, errCode)
	}

	var result PKCEResult
	body := map[string]string{"challenge_id": challenge.ID, "code": code}
	status, errCode, err = supabaseJSON(http.MethodPost, supabaseURL, base+"/verify", supabaseAnonKey, accessToken, body, &result)
	if err != nil {
		return nil, fmt.Errorf("two-factor verify: %w", err)
	}
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return nil, ErrWrongCode
	}
	if status != http.StatusOK || result.AccessToken == "" {
		return nil, fmt.Errorf("two-factor verify failed (%d%s)", status, errCode)
	}
	return &result, nil
}

// CompleteMFA upgrades a fresh login to aal2 when the user has a verified TOTP
// factor. Without one it returns the login tokens unchanged and never calls
// prompt. Each attempt gets its own challenge; after maxCodeAttempts wrong
// codes it returns an error.
func CompleteMFA(supabaseURL, supabaseAnonKey string, login *PKCEResult, prompt CodePrompt) (*PKCEResult, error) {
	factorID, err := VerifiedTOTPFactor(supabaseURL, supabaseAnonKey, login.AccessToken)
	if err != nil {
		return nil, err
	}
	if factorID == "" {
		return login, nil
	}
	for attempt := 1; attempt <= maxCodeAttempts; attempt++ {
		code, err := prompt(attempt)
		if err != nil {
			return nil, err
		}
		result, err := VerifyTOTP(supabaseURL, supabaseAnonKey, login.AccessToken, factorID, strings.TrimSpace(code))
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrWrongCode) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("two-factor code was wrong %d times — run `weside auth login` again", maxCodeAttempts)
}

// supabaseJSON sends one JSON request to the Supabase auth API with the
// session's bearer token and decodes a 200 body into result. It returns the
// HTTP status and, for an error status, Supabase's `error_code` formatted as
// ": <code>" (empty when the body carries none) so messages can name it.
func supabaseJSON(method, supabaseURL, path, anonKey, accessToken string, body, result any) (int, string, error) {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, "", err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, strings.TrimRight(supabaseURL, "/")+path, reader)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("apikey", anonKey)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := supabaseHTTPClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			ErrorCode string `json:"error_code"`
		}
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.ErrorCode != "" {
			return resp.StatusCode, ": " + e.ErrorCode, nil
		}
		return resp.StatusCode, "", nil
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return resp.StatusCode, "", fmt.Errorf("parsing response: %w", err)
		}
	}
	return resp.StatusCode, "", nil
}
