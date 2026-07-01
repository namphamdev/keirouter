package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/go-chi/chi/v5"
	"github.com/mydisha/keirouter/backend/internal/connectors"
)

// defaultQuotaScriptTimeout caps how long the embedded JS engine may run.
const defaultQuotaScriptTimeout = 30 * time.Second

// maxQuotaScriptBytes limits user-provided script size to prevent abuse.
const maxQuotaScriptBytes = 16 * 1024

// quotaCheckResult is the JSON shape returned by the check-quota endpoint.
type quotaCheckResult struct {
	Ok      bool            `json:"ok"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
	Elapsed int64           `json:"elapsed_ms"`
}

// runQuotaScript executes the user-supplied JavaScript against an account's
// decrypted API key. The script receives a global fetch() compatible with the
// browser Fetch API (returning {status, headers, json(), text()}) and a global
// API_KEY string. The script must return a JSON-serialisable value (object,
// string, number, etc.) which is forwarded to the caller.
//
// All network egress is server-side (the Go process issues the HTTP request),
// so this never leaks the API key to the browser.
func runQuotaScript(ctx context.Context, scriptSrc, apiKey string) quotaCheckResult {
	start := time.Now()

	if len(scriptSrc) > maxQuotaScriptBytes {
		return quotaCheckResult{Ok: false, Error: fmt.Sprintf("script too large (max %d bytes)", maxQuotaScriptBytes), Elapsed: time.Since(start).Milliseconds()}
	}

	vm := goja.New()
	vm.SetFieldNameMapper(goja.UncapFieldNameMapper())

	// fetch implements a subset of the browser fetch() API.
	fetchFn := func(call goja.FunctionCall) goja.Value {
		urlStr := ""
		opts := map[string]any{}
		if len(call.Arguments) > 0 {
			urlStr = call.Argument(0).String()
		}
		if len(call.Arguments) > 1 && call.Argument(1) != goja.Undefined() && call.Argument(1) != goja.Null() {
			opts = call.Argument(1).Export().(map[string]any)
		}

		method := "GET"
		if m, ok := opts["method"].(string); ok && m != "" {
			method = strings.ToUpper(m)
		}

		var body io.Reader
		if b, ok := opts["body"]; ok && b != nil {
			body = strings.NewReader(fmt.Sprint(b))
		}

		headers := map[string]string{}
		if h, ok := opts["headers"].(map[string]any); ok {
			for k, v := range h {
				headers[k] = fmt.Sprint(v)
			}
		}

		reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, method, urlStr, body)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("fetch: invalid request: %s", err.Error())))
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("fetch: request failed: %s", err.Error())))
		}

		// Build a response object that mirrors the browser Response interface.
		responseObj := vm.NewObject()
		responseObj.Set("status", resp.StatusCode)
		responseObj.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
		responseObj.Set("statusText", resp.Status)

		// headers as a plain object
		headerMap := map[string]string{}
		for k := range resp.Header {
			headerMap[strings.ToLower(k)] = resp.Header.Get(k)
		}
		responseObj.Set("headers", headerMap)

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		responseObj.Set("text", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
					fn(goja.Undefined(), vm.ToValue(string(bodyBytes)))
					return goja.Undefined()
				}
			}
			return vm.ToValue(string(bodyBytes))
		})

		responseObj.Set("json", func(call goja.FunctionCall) goja.Value {
			var parsed any
			if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
				errMsg := fmt.Sprintf("json: invalid response body (%d bytes): %s", len(bodyBytes), err.Error())
				if len(call.Arguments) > 0 {
					if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
						fn(vm.ToValue(errMsg), goja.Undefined())
						return goja.Undefined()
					}
				}
				panic(vm.ToValue(errMsg))
			}
			if len(call.Arguments) > 0 {
				if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
					fn(goja.Undefined(), vm.ToValue(parsed))
					return goja.Undefined()
				}
			}
			return vm.ToValue(parsed)
		})

		return responseObj
	}
	vm.Set("fetch", fetchFn)
	vm.Set("API_KEY", apiKey)
	// console.log for debugging convenience; captured in output.
	logLines := []string{}
	vm.Set("console", map[string]any{
		"log":   func(call goja.FunctionCall) goja.Value { return consoleLog(vm, &logLines, call) },
		"warn":  func(call goja.FunctionCall) goja.Value { return consoleLog(vm, &logLines, call) },
		"error": func(call goja.FunctionCall) goja.Value { return consoleLog(vm, &logLines, call) },
		"info":  func(call goja.FunctionCall) goja.Value { return consoleLog(vm, &logLines, call) },
	})

	// Wrapper so the user script can use async/await. We wrap their code in an
	// async IIFE and await the result.
	wrapped := fmt.Sprintf(`
		var __resolve, __reject;
		var __promise = new Promise(function(resolve, reject) {
			__resolve = resolve;
			__reject = reject;
		});
		(async function() {
			try {
				var __result = (function() {
					%s
				})();
				__resolve(__result);
			} catch(e) {
				__reject(e);
			}
		})();
		__promise;
	`, scriptSrc)

	// Set a deadline goroutine.
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(defaultQuotaScriptTimeout):
			vm.Interrupt("quota script timeout")
		case <-done:
		}
	}()
	defer close(done)

	// Execute wrapper to get the promise.
	val, err := vm.RunString(wrapped)
	if err != nil {
		return quotaCheckResult{Ok: false, Error: "script error: " + err.Error(), Elapsed: time.Since(start).Milliseconds()}
	}

	promise := val.Export()
	p, ok := promise.(*goja.Promise)
	if !ok {
		// Synchronous result (script didn't return a promise).
		return resolveQuotaResult(vm, val, logLines, start)
	}

	// Wait for promise to settle.
	resultCh := make(chan goja.Value, 1)
	errCh := make(chan string, 1)
	go func() {
		for {
			switch p.State() {
			case goja.PromiseStateFulfilled:
				resultCh <- p.Result()
				return
			case goja.PromiseStateRejected:
				errCh <- p.Result().String()
				return
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	select {
	case rv := <-resultCh:
		return resolveQuotaResult(vm, rv, logLines, start)
	case errMsg := <-errCh:
		return quotaCheckResult{Ok: false, Error: "script rejected: " + errMsg, Elapsed: time.Since(start).Milliseconds()}
	case <-time.After(defaultQuotaScriptTimeout):
		return quotaCheckResult{Ok: false, Error: "script timed out", Elapsed: time.Since(start).Milliseconds()}
	}
}

func consoleLog(vm *goja.Runtime, lines *[]string, call goja.FunctionCall) goja.Value {
	parts := make([]string, 0, len(call.Arguments))
	for _, arg := range call.Arguments {
		if arg == goja.Undefined() || arg == goja.Null() {
			parts = append(parts, arg.String())
			continue
		}
		exported := arg.Export()
		raw, err := json.Marshal(exported)
		if err != nil {
			parts = append(parts, arg.String())
		} else {
			parts = append(parts, string(raw))
		}
	}
	*lines = append(*lines, strings.Join(parts, " "))
	return goja.Undefined()
}

func resolveQuotaResult(vm *goja.Runtime, val goja.Value, logLines []string, start time.Time) quotaCheckResult {
	exported := val.Export()

	// If the script returned undefined or null, include console output.
	if exported == nil {
		output := map[string]any{}
		if len(logLines) > 0 {
			output["console"] = logLines
		}
		raw, _ := json.Marshal(output)
		return quotaCheckResult{Ok: true, Output: raw, Elapsed: time.Since(start).Milliseconds()}
	}

	raw, err := json.Marshal(exported)
	if err != nil {
		return quotaCheckResult{Ok: false, Error: "result not JSON-serialisable: " + err.Error(), Elapsed: time.Since(start).Milliseconds()}
	}

	// If result is a plain string/number, wrap with console output.
	if len(logLines) > 0 {
		var wrapper map[string]any
		if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper != nil {
			wrapper["console"] = logLines
			raw, _ = json.Marshal(wrapper)
		}
	}

	return quotaCheckResult{Ok: true, Output: raw, Elapsed: time.Since(start).Milliseconds()}
}

// ---- HTTP handlers ----------------------------------------------------------

// providerQuotaScriptPrefix is the settings-store key prefix for per-provider
// quota check JavaScript.
const providerQuotaScriptPrefix = "provider_quota_script_"

// quotaScriptResponse is returned by GET /providers/{id}/quota-script.
type quotaScriptResponse struct {
	Provider string `json:"provider"`
	Script   string `json:"script"`
}

// adminGetQuotaScript returns the stored JavaScript for a provider's quota check.
func (s *Server) adminGetQuotaScript(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "id")
	if _, ok := connectors.SpecByID(provider); !ok {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	script := ""
	if s.settings != nil {
		if raw, err := s.settings.Get(r.Context(), providerQuotaScriptPrefix+provider); err == nil {
			script = raw
		}
	}
	writeJSON(w, http.StatusOK, quotaScriptResponse{Provider: provider, Script: script})
}

// adminUpdateQuotaScript stores the JavaScript for a provider's quota check.
func (s *Server) adminUpdateQuotaScript(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings store not configured")
		return
	}
	provider := chi.URLParam(r, "id")
	if _, ok := connectors.SpecByID(provider); !ok {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	var body struct {
		Script string `json:"script"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Script) > maxQuotaScriptBytes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("script too large (max %d bytes)", maxQuotaScriptBytes))
		return
	}
	if err := s.settings.Set(r.Context(), providerQuotaScriptPrefix+provider, body.Script); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, quotaScriptResponse{Provider: provider, Script: body.Script})
}

// adminCheckQuotaScript executes the provider's stored JavaScript quota check
// for a specific account, replacing API_KEY with the account's decrypted key.
func (s *Server) adminCheckQuotaScript(w http.ResponseWriter, r *http.Request) {
	accID := chi.URLParam(r, "id")
	acc, err := s.accounts.Get(r.Context(), accID)
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if s.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings store not configured")
		return
	}
	script, err := s.settings.Get(r.Context(), providerQuotaScriptPrefix+acc.Provider)
	if err != nil || strings.TrimSpace(script) == "" {
		writeError(w, http.StatusNotFound, "no quota check script configured for this provider")
		return
	}

	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not configured")
		return
	}
	creds, err := s.vault.Open(acc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not decrypt credentials")
		return
	}

	// Build the API key string. For OAuth accounts the access token acts as the key.
	apiKey := creds.APIKey
	if apiKey == "" && creds.AccessToken != "" {
		apiKey = creds.AccessToken
	}

	result := runQuotaScript(r.Context(), script, apiKey)
	writeJSON(w, http.StatusOK, result)
}
