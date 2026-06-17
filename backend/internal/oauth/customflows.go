package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// This file implements provider connect flows that do not fit the standard
// authorization-code or device-code grants: KiloCode's custom device-auth poll,
// Qoder's PKCE device-token poll, CodeBuddy's browser-poll, and Cursor's
// import-token validation. Each is small and self-contained, mirroring the
// shape of the generic flow helpers (DeviceCode / PollResult / Tokens).

// ---------------------------------------------------------------------------
// KiloCode (custom device-auth)
// ---------------------------------------------------------------------------

const (
	kilocodeAPIBase     = "https://api.kilo.ai"
	kilocodeInitiateURL = "https://api.kilo.ai/api/device-auth/codes"
)

// KilocodeStartDeviceAuth initiates a KiloCode device-auth request. The upstream
// issues a single code that doubles as the user code and the poll key.
func KilocodeStartDeviceAuth(ctx context.Context) (*DeviceCode, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kilocodeInitiateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("kilocode: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kilocode: request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("kilocode: too many pending authorization requests; try again later")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kilocode: device auth init failed (%d): %s", resp.StatusCode, truncate(raw, 300))
	}
	var parsed struct {
		Code            string `json:"code"`
		VerificationURL string `json:"verificationUrl"`
		ExpiresIn       int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("kilocode: parse device auth response: %w", err)
	}
	if parsed.Code == "" {
		return nil, fmt.Errorf("kilocode: device auth response missing code")
	}
	expires := parsed.ExpiresIn
	if expires <= 0 {
		expires = 300
	}
	return &DeviceCode{
		DeviceCode:              parsed.Code,
		UserCode:                parsed.Code,
		VerificationURI:         parsed.VerificationURL,
		VerificationURIComplete: parsed.VerificationURL,
		ExpiresIn:               expires,
		Interval:                3,
	}, nil
}

// KilocodePollToken polls the KiloCode device-auth status endpoint. On approval
// it returns the bearer token plus best-effort org id and email.
func KilocodePollToken(ctx context.Context, code string) PollResult {
	pollURL := kilocodeInitiateURL + "/" + url.PathEscape(code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return PollResult{Err: fmt.Errorf("kilocode: build poll request: %w", err)}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return PollResult{Err: fmt.Errorf("kilocode: poll request: %w", err)}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted:
		return PollResult{Pending: true}
	case http.StatusForbidden:
		return PollResult{Err: fmt.Errorf("kilocode: authorization denied by user")}
	case http.StatusGone:
		return PollResult{Err: fmt.Errorf("kilocode: authorization code expired")}
	}
	if resp.StatusCode >= 400 {
		return PollResult{Err: fmt.Errorf("kilocode: poll failed: %d", resp.StatusCode)}
	}

	raw, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Status    string `json:"status"`
		Token     string `json:"token"`
		UserEmail string `json:"userEmail"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return PollResult{Err: fmt.Errorf("kilocode: parse poll response: %w", err)}
	}
	if parsed.Status != "approved" || parsed.Token == "" {
		return PollResult{Pending: true}
	}

	tokens := &Tokens{AccessToken: parsed.Token, Email: parsed.UserEmail}
	if orgID := kilocodeOrgID(ctx, parsed.Token); orgID != "" {
		tokens.Extra = map[string]string{"org_id": orgID}
	}
	return PollResult{Done: true, Tokens: tokens}
}

// kilocodeOrgID fetches the first organization id for the token (best effort).
func kilocodeOrgID(ctx context.Context, token string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kilocodeAPIBase+"/api/profile", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var profile struct {
		Organizations []struct {
			ID string `json:"id"`
		} `json:"organizations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return ""
	}
	if len(profile.Organizations) > 0 {
		return profile.Organizations[0].ID
	}
	return ""
}

// ---------------------------------------------------------------------------
// Qoder (PKCE device-token poll)
// ---------------------------------------------------------------------------

const (
	qoderLoginURL       = "https://qoder.com/device/selectAccounts"
	qoderDeviceTokenURL = "https://openapi.qoder.sh/api/v1/deviceToken/poll"
	qoderUserInfoURL    = "https://openapi.qoder.sh/api/v1/userinfo"
	qoderUserAgent      = "Go-http-client/2.0"
)

// QoderDeviceFlow is the local state of a Qoder device-token flow. The poll
// endpoint identifies the device by nonce + PKCE verifier rather than a
// server-issued device code.
type QoderDeviceFlow struct {
	Nonce                   string
	Verifier                string
	MachineID               string
	VerificationURIComplete string
}

// QoderInitiateDeviceFlow generates the PKCE pair, nonce, and machine id and
// builds the browser verification URL.
func QoderInitiateDeviceFlow() (*QoderDeviceFlow, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("qoder: read random: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	nonce := uuid.NewString()
	machineID := uuid.NewString()

	q := url.Values{}
	q.Set("challenge", challenge)
	q.Set("challenge_method", "S256")
	q.Set("machine_id", machineID)
	q.Set("nonce", nonce)

	return &QoderDeviceFlow{
		Nonce:                   nonce,
		Verifier:                verifier,
		MachineID:               machineID,
		VerificationURIComplete: qoderLoginURL + "?" + q.Encode(),
	}, nil
}

// QoderPollToken polls the Qoder device-token endpoint once. 202/404 mean the
// user has not finished the browser flow yet.
func QoderPollToken(ctx context.Context, nonce, verifier, machineID string) PollResult {
	if nonce == "" || verifier == "" {
		return PollResult{Err: fmt.Errorf("qoder: missing nonce or verifier")}
	}
	q := url.Values{}
	q.Set("nonce", nonce)
	q.Set("verifier", verifier)
	q.Set("challenge_method", "S256")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qoderDeviceTokenURL+"?"+q.Encode(), nil)
	if err != nil {
		return PollResult{Err: fmt.Errorf("qoder: build poll request: %w", err)}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", qoderUserAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return PollResult{Err: fmt.Errorf("qoder: poll request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNotFound {
		return PollResult{Pending: true}
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return PollResult{Err: fmt.Errorf("qoder: poll failed (%d): %s", resp.StatusCode, truncate(raw, 300))}
	}

	var parsed struct {
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
		ExpiresAt    any    `json:"expires_at"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return PollResult{Err: fmt.Errorf("qoder: parse poll response: %w", err)}
	}
	if parsed.Token == "" {
		return PollResult{Err: fmt.Errorf("qoder: poll returned 200 but no token")}
	}

	expiresIn := qoderExpiresIn(parsed.ExpiresAt, parsed.ExpiresIn)
	tokens := &Tokens{
		AccessToken:  parsed.Token,
		RefreshToken: parsed.RefreshToken,
		ExpiresIn:    expiresIn,
		Extra: map[string]string{
			"auth_method": "device",
			"user_id":     parsed.UserID,
			"machine_id":  machineID,
		},
	}
	qoderFetchUserInfo(ctx, tokens)
	if tokens.Email == "" && parsed.UserID != "" {
		tokens.Email = "qoder-user-" + parsed.UserID
	}
	return PollResult{Done: true, Tokens: tokens}
}

// qoderExpiresIn normalizes Qoder's expiry hint to seconds-from-now, flooring at
// one day and defaulting to 30 days when the upstream omits expiry.
func qoderExpiresIn(expiresAt any, expiresInSeconds int) int {
	const day = 24 * 60 * 60
	switch v := expiresAt.(type) {
	case float64:
		if v > 0 {
			remaining := int(int64(v)/1000 - time.Now().Unix())
			if remaining > day {
				return remaining
			}
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
				remaining := int(time.Until(t).Seconds())
				if remaining > day {
					return remaining
				}
			}
		}
	}
	if expiresInSeconds > day {
		return expiresInSeconds
	}
	return 30 * day
}

// qoderFetchUserInfo populates the account label from the Qoder profile.
func qoderFetchUserInfo(ctx context.Context, t *Tokens) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qoderUserInfoURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", qoderUserAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return
	}
	var info struct {
		Name           string `json:"name"`
		Username       string `json:"username"`
		Email          string `json:"email"`
		OrganizationID string `json:"organization_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return
	}
	t.Email = strings.TrimSpace(info.Email)
	t.DisplayName = strings.TrimSpace(info.Name)
	if t.DisplayName == "" {
		t.DisplayName = strings.TrimSpace(info.Username)
	}
	if info.OrganizationID != "" {
		t.Extra["organization_id"] = strings.TrimSpace(info.OrganizationID)
	}
}

// ---------------------------------------------------------------------------
// CodeBuddy (browser-poll)
// ---------------------------------------------------------------------------

const (
	codebuddyStateURL  = "https://copilot.tencent.com/v2/plugin/auth/state"
	codebuddyTokenURL  = "https://copilot.tencent.com/v2/plugin/auth/token"
	codebuddyUserAgent = "CLI/2.63.2 CodeBuddy/2.63.2"
)

// codebuddyHeaders are sent on every CodeBuddy auth request.
func codebuddyHeaders() map[string]string {
	return map[string]string{
		"Content-Type":      "application/json",
		"Accept":            "application/json",
		"User-Agent":        codebuddyUserAgent,
		"X-Requested-With":  "XMLHttpRequest",
		"X-Domain":          "copilot.tencent.com",
		"X-No-Authorization": "true",
		"X-No-User-Id":      "true",
		"X-Product":         "SaaS",
	}
}

// CodebuddyStartAuth requests a login state + browser auth URL. The state
// doubles as the poll key.
func CodebuddyStartAuth(ctx context.Context) (*DeviceCode, error) {
	raw, status, err := codebuddyHTTP(ctx, codebuddyStateURL+"?platform=CLI", []byte("{}"))
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("codebuddy: state request failed (%d): %s", status, truncate(raw, 300))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			State   string `json:"state"`
			AuthURL string `json:"authUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("codebuddy: parse state response: %w", err)
	}
	if parsed.Code != 0 || parsed.Data.State == "" || parsed.Data.AuthURL == "" {
		msg := parsed.Msg
		if msg == "" {
			msg = "missing state/authUrl"
		}
		return nil, fmt.Errorf("codebuddy: state error: %s", msg)
	}
	return &DeviceCode{
		DeviceCode:              parsed.Data.State,
		VerificationURI:         parsed.Data.AuthURL,
		VerificationURIComplete: parsed.Data.AuthURL,
		ExpiresIn:               300,
		Interval:                5,
	}, nil
}

// CodebuddyPollToken polls for the access token. Upstream code 11217 means the
// user has not authorized yet; code 0 with a token means success.
func CodebuddyPollToken(ctx context.Context, state string) PollResult {
	body, _ := json.Marshal(map[string]string{"state": state})
	raw, status, err := codebuddyHTTP(ctx, codebuddyTokenURL, body)
	if err != nil {
		return PollResult{Err: err}
	}
	if status >= 400 {
		return PollResult{Err: fmt.Errorf("codebuddy: poll failed (%d)", status)}
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return PollResult{Err: fmt.Errorf("codebuddy: parse poll response: %w", err)}
	}
	if parsed.Code == 0 && parsed.Data.AccessToken != "" {
		return PollResult{Done: true, Tokens: &Tokens{
			AccessToken:  parsed.Data.AccessToken,
			RefreshToken: parsed.Data.RefreshToken,
			ExpiresIn:    86400,
		}}
	}
	if parsed.Code == 11217 {
		return PollResult{Pending: true}
	}
	msg := parsed.Msg
	if msg == "" {
		msg = "unknown error"
	}
	return PollResult{Err: fmt.Errorf("codebuddy: %s", msg)}
}

// codebuddyHTTP posts a JSON body to a CodeBuddy auth endpoint.
func codebuddyHTTP(ctx context.Context, endpoint string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("codebuddy: build request: %w", err)
	}
	for k, v := range codebuddyHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("codebuddy: request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("codebuddy: read response: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Cursor (import token)
// ---------------------------------------------------------------------------

// CursorImportToken validates a token pasted from the Cursor IDE and returns it
// as an access token. Cursor has no public refresh endpoint, so the token is
// stored as-is with a derived machine id. A best-effort decode extracts the
// account subject for the label.
func CursorImportToken(ctx context.Context, token string) (*Tokens, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("cursor: token is required")
	}

	machineID := cursorMachineID(token)
	tokens := &Tokens{
		AccessToken: token,
		ExpiresIn:   86400,
		Extra: map[string]string{
			"machine_id":  machineID,
			"auth_method": "imported",
		},
	}
	if payload := decodeJWTPayload(token); payload != nil {
		if email, _ := payload["email"].(string); email != "" {
			tokens.Email = email
		}
		if sub, _ := payload["sub"].(string); sub != "" && tokens.Email == "" {
			tokens.Email = sub
		}
	}
	return tokens, nil
}

// cursorMachineID derives a stable machine id from the token, matching the
// Cursor connector's expectation when none is supplied explicitly.
func cursorMachineID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Cursor (deep-control PKCE login + poll + refresh)
// ---------------------------------------------------------------------------

const (
	cursorLoginURL   = "https://cursor.com/loginDeepControl"
	cursorPollURL    = "https://api2.cursor.sh/auth/poll"
	cursorRefreshURL = "https://api2.cursor.sh/auth/exchange_user_api_key"
)

// CursorLoginFlow is the local state of a Cursor deep-control login. The poll
// endpoint identifies the pending login by the generated uuid plus the PKCE
// verifier rather than a server-issued device code.
type CursorLoginFlow struct {
	UUID                    string
	Verifier                string
	VerificationURIComplete string
}

// CursorInitiateLogin generates the PKCE pair + uuid and builds the browser
// login URL Cursor opens for "deep control" CLI authentication.
func CursorInitiateLogin() (*CursorLoginFlow, error) {
	pkce, err := GeneratePKCE(96)
	if err != nil {
		return nil, fmt.Errorf("cursor: generate pkce: %w", err)
	}
	id := uuid.NewString()

	q := url.Values{}
	q.Set("challenge", pkce.Challenge)
	q.Set("uuid", id)
	q.Set("mode", "login")
	q.Set("redirectTarget", "cli")

	return &CursorLoginFlow{
		UUID:                    id,
		Verifier:                pkce.Verifier,
		VerificationURIComplete: cursorLoginURL + "?" + q.Encode(),
	}, nil
}

// CursorPollToken polls the Cursor auth poll endpoint once. A 404 means the
// user has not finished the browser login yet; a 200 with an accessToken means
// success.
func CursorPollToken(ctx context.Context, loginUUID, verifier string) PollResult {
	if loginUUID == "" || verifier == "" {
		return PollResult{Err: fmt.Errorf("cursor: missing uuid or verifier")}
	}
	q := url.Values{}
	q.Set("uuid", loginUUID)
	q.Set("verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cursorPollURL+"?"+q.Encode(), nil)
	if err != nil {
		return PollResult{Err: fmt.Errorf("cursor: build poll request: %w", err)}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return PollResult{Err: fmt.Errorf("cursor: poll request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return PollResult{Pending: true}
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return PollResult{Err: fmt.Errorf("cursor: poll failed (%d): %s", resp.StatusCode, truncate(raw, 300))}
	}

	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return PollResult{Err: fmt.Errorf("cursor: parse poll response: %w", err)}
	}
	if parsed.AccessToken == "" {
		return PollResult{Pending: true}
	}
	return PollResult{Done: true, Tokens: cursorBuildTokens(parsed.AccessToken, parsed.RefreshToken)}
}

// CursorRefresh exchanges a Cursor refresh token for a fresh access token via
// the exchange_user_api_key endpoint. Cursor returns a new access token and,
// when rotated, a new refresh token.
func CursorRefresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("cursor: no refresh token available")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cursorRefreshURL, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("cursor: build refresh request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor: refresh request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, classifyRefreshError(raw, resp.StatusCode)
	}

	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("cursor: parse refresh response: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("cursor: refresh response missing accessToken")
	}
	tokens := cursorBuildTokens(parsed.AccessToken, parsed.RefreshToken)
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}
	return tokens, nil
}

// cursorBuildTokens normalizes a Cursor access/refresh pair into Tokens,
// deriving the access-token lifetime, account label, and machine id.
func cursorBuildTokens(accessToken, refreshToken string) *Tokens {
	tokens := &Tokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    cursorTokenExpiresIn(accessToken),
		Extra: map[string]string{
			"machine_id":  cursorMachineID(accessToken),
			"auth_method": "oauth",
		},
	}
	if payload := decodeJWTPayload(accessToken); payload != nil {
		if email, _ := payload["email"].(string); email != "" {
			tokens.Email = email
		}
		if sub, _ := payload["sub"].(string); sub != "" && tokens.Email == "" {
			tokens.Email = sub
		}
	}
	return tokens
}

// cursorTokenExpiresIn derives the access-token lifetime in seconds from the
// JWT exp claim, defaulting to one day when the claim is missing.
func cursorTokenExpiresIn(token string) int {
	if payload := decodeJWTPayload(token); payload != nil {
		if exp, ok := payload["exp"].(float64); ok && exp > 0 {
			if remaining := int(int64(exp) - time.Now().Unix()); remaining > 0 {
				return remaining
			}
		}
	}
	return 86400
}

// ---------------------------------------------------------------------------
// Command Code (import token)
// ---------------------------------------------------------------------------

// CommandCodeImportToken validates a token pasted from the Command Code CLI
// (~/.commandcode/auth.json) or generated at commandcode.ai/studio. The token
// is stored as an access token; the CommandCode connector sends it as a Bearer
// token on the /alpha/generate endpoint. A best-effort JWT decode extracts the
// user email for the account label.
func CommandCodeImportToken(_ context.Context, token string) (*Tokens, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("commandcode: token is required")
	}

	tokens := &Tokens{
		AccessToken: token,
		ExpiresIn:   86400 * 30,
		Extra: map[string]string{
			"auth_method": "imported",
		},
	}
	if payload := decodeJWTPayload(token); payload != nil {
		if email, _ := payload["email"].(string); email != "" {
			tokens.Email = email
		}
		if sub, _ := payload["sub"].(string); sub != "" && tokens.Email == "" {
			tokens.Email = sub
		}
	}
	return tokens, nil
}