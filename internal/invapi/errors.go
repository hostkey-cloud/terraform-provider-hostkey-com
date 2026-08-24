package invapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	secretJSONRe = regexp.MustCompile(`(?i)("(?:token|key|password|root_pass|passphrase|secret)"\s*:\s*")[^"]*(")`)
	secretFormRe = regexp.MustCompile(`(?i)((?:^|&)(?:token|key|password|root_pass)=)[^&]*`)
)

// APIError is returned when InvAPI responds with a business error envelope.
type APIError struct {
	Code    int    // numeric code when present (result=-1, code=1, …)
	Name    string // string code when present (e.g. NO_APPROPRIATE_SERVERS)
	Message string
	Result  string
	Body    string
}

func (e *APIError) Error() string {
	switch {
	case e.Name != "" && e.Message != "":
		return fmt.Sprintf("invapi error (%s): %s", e.Name, redactSecrets(e.Message))
	case e.Message != "":
		if e.Code != 0 {
			return fmt.Sprintf("invapi error (code=%d): %s", e.Code, redactSecrets(e.Message))
		}
		return fmt.Sprintf("invapi error: %s", redactSecrets(e.Message))
	case e.Result != "" && e.Result != "OK":
		return fmt.Sprintf("invapi error: result=%s", redactSecrets(e.Result))
	default:
		return fmt.Sprintf("invapi error: %s", redactSecrets(e.Body))
	}
}

// IsNotFound reports whether err means the InvAPI object is gone (safe to drop
// Terraform state). Matches ErrNotFound and common InvAPI business envelopes.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var api *APIError
	if errors.As(err, &api) {
		blob := strings.ToLower(strings.TrimSpace(api.Message + " " + api.Result + " " + api.Body))
		return strings.Contains(blob, "not found") ||
			strings.Contains(blob, "no such") ||
			strings.Contains(blob, "unknown server") ||
			strings.Contains(blob, "server not exist") ||
			strings.Contains(blob, "does not exist")
	}
	return false
}

func decodeAPIError(body []byte) error {
	var envelope struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Error   string          `json:"error"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("invapi: invalid JSON response: %w; body: %s", err, truncate(body, 512))
	}

	name := codeAsName(envelope.Code)
	if resultErr := resultFieldError(envelope.Result); resultErr != nil {
		if apiErr, ok := resultErr.(*APIError); ok {
			if apiErr.Message == "" && envelope.Error != "" {
				apiErr.Message = redactSecrets(envelope.Error)
			}
			if apiErr.Name == "" {
				apiErr.Name = name
			}
			if apiErr.Body == "" || apiErr.Body == string(envelope.Result) {
				apiErr.Body = redactSecrets(string(body))
			}
		}
		return resultErr
	}

	msg := envelope.Message
	if msg == "" {
		msg = envelope.Error
	}
	msg = redactSecrets(msg)
	if len(envelope.Code) > 0 && string(envelope.Code) != "0" && string(envelope.Code) != "null" {
		return &APIError{
			Message: msg,
			Body:    redactSecrets(string(body)),
			Code:    codeAsInt(envelope.Code),
			Name:    name,
		}
	}
	if msg != "" {
		return &APIError{Message: msg, Body: redactSecrets(string(body)), Name: name}
	}

	return nil
}

func resultFieldError(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		// InvAPI uses "OK"; some WHMCS endpoints use "success".
		if s == "OK" || s == "" || strings.EqualFold(s, "success") {
			return nil
		}
		return &APIError{Result: redactSecrets(s), Body: redactSecrets(string(raw))}
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		if int(n) == 0 || int(n) == 1 {
			return nil
		}
		return &APIError{Code: int(n), Body: redactSecrets(string(raw))}
	}
	return nil
}

func codeAsInt(raw json.RawMessage) int {
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	return -1
}

// codeAsName returns a string InvAPI error code (e.g. "NO_APPROPRIATE_SERVERS").
// Numeric codes are left empty; use Code instead.
func codeAsName(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// IsNoAppropriateServers reports InvAPI auth/login refusal when the account
// has zero servers (code NO_APPROPRIATE_SERVERS / "No appropriate servers found").
func IsNoAppropriateServers(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNoAppropriateServers) {
		return true
	}
	var api *APIError
	if errors.As(err, &api) {
		if strings.EqualFold(api.Name, "NO_APPROPRIATE_SERVERS") {
			return true
		}
		blob := strings.ToLower(api.Message + " " + api.Result + " " + api.Body)
		if strings.Contains(blob, "no appropriate servers") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "no appropriate servers")
}

func truncate(b []byte, n int) string {
	s := redactSecrets(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func redactSecrets(s string) string {
	s = secretJSONRe.ReplaceAllString(s, `${1}***${2}`)
	s = secretFormRe.ReplaceAllString(s, `${1}***`)
	return s
}
