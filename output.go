package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const jsonSchemaVersion = 1

type jsonEnvelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Action        string     `json:"action"`
	Result        any        `json:"result,omitempty"`
	Error         *jsonError `json:"error,omitempty"`
}

type jsonError struct {
	Code               string `json:"code"`
	Message            string `json:"message"`
	Suggestion         string `json:"suggestion,omitempty"`
	Retryable          bool   `json:"retryable"`
	RequiresUserAction bool   `json:"requires_user_action"`
}

type commandError struct {
	code       string
	suggestion string
	err        error
}

func (e commandError) Error() string { return e.err.Error() }
func (e commandError) Unwrap() error { return e.err }

func withErrorCode(code string, err error, suggestion string) error {
	if err == nil {
		return nil
	}
	return commandError{code: code, suggestion: suggestion, err: err}
}

func writeJSONSuccess(action string, result any) error {
	return writeJSON(jsonEnvelope{
		SchemaVersion: jsonSchemaVersion,
		OK:            true,
		Action:        action,
		Result:        result,
	})
}

func writeJSONFailure(action string, err error) error {
	code := "internal_error"
	suggestion := "Run wadb doctor --output json for diagnostics."

	var invalidUsage usageError
	if errors.As(err, &invalidUsage) {
		code = "invalid_arguments"
		suggestion = "Run wadb --help to inspect the supported command syntax."
	}
	var commandErr commandError
	if errors.As(err, &commandErr) {
		code = commandErr.code
		suggestion = commandErr.suggestion
	}
	retryable, requiresUserAction := errorTraits(code)

	return writeJSON(jsonEnvelope{
		SchemaVersion: jsonSchemaVersion,
		OK:            false,
		Action:        action,
		Error: &jsonError{
			Code:               code,
			Message:            err.Error(),
			Suggestion:         suggestion,
			Retryable:          retryable,
			RequiresUserAction: requiresUserAction,
		},
	})
}

func errorTraits(code string) (retryable, requiresUserAction bool) {
	switch code {
	case "adb_error", "disconnect_failed":
		return true, false
	case "discovery_timeout", "connection_failed":
		return true, true
	case "interactive_required", "invalid_pairing_code", "adb_not_found", "pairing_failed", "device_not_found", "multiple_devices":
		return false, true
	default:
		return false, false
	}
}

func knownErrorCapabilities() []errorCapability {
	descriptions := []struct {
		code        string
		description string
	}{
		{"invalid_arguments", "The command, flags, or environment values are invalid."},
		{"interactive_required", "The requested flow needs terminal input or a physical user action."},
		{"invalid_pairing_code", "Standard input did not contain exactly six digits."},
		{"adb_not_found", "Android platform-tools could not be located."},
		{"adb_error", "The local adb installation or server failed."},
		{"pairing_failed", "Android rejected or could not complete pairing."},
		{"discovery_timeout", "No suitable Wireless debugging mDNS announcement appeared."},
		{"device_not_found", "No device matches the supplied selector."},
		{"multiple_devices", "The command requires an explicit device selection."},
		{"connection_failed", "adb could not connect to the selected endpoint."},
		{"disconnect_failed", "adb could not disconnect the selected endpoint."},
		{"internal_error", "An unexpected failure has no more specific classification."},
	}
	result := make([]errorCapability, 0, len(descriptions))
	for _, item := range descriptions {
		retryable, requiresUserAction := errorTraits(item.code)
		result = append(result, errorCapability{Code: item.code, Description: item.description, Retryable: retryable, RequiresUserAction: requiresUserAction})
	}
	return result
}

func writeJSON(value any) error {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}
