package main

import (
	"fmt"
	"os"
	"text/tabwriter"
)

type capabilitiesResult struct {
	CLIVersion           string              `json:"cli_version"`
	SchemaVersion        int                 `json:"schema_version"`
	SchemaFile           string              `json:"schema_file"`
	SchemaCommand        string              `json:"schema_command"`
	StructuredOutputFlag string              `json:"structured_output_flag"`
	NonInteractiveFlag   string              `json:"non_interactive_flag"`
	LegacyJSONFlag       string              `json:"legacy_json_flag"`
	Commands             []commandCapability `json:"commands"`
	Errors               []errorCapability   `json:"errors"`
}

type commandCapability struct {
	Name                     string `json:"name"`
	Invocation               string `json:"invocation"`
	Description              string `json:"description"`
	MutatesState             bool   `json:"mutates_state"`
	RequiresUserAction       bool   `json:"requires_user_action"`
	RequiresStdin            bool   `json:"requires_stdin"`
	SupportsStructuredOutput bool   `json:"supports_structured_output"`
	DeviceSelection          string `json:"device_selection,omitempty"`
	AllowsAll                bool   `json:"allows_all"`
}

type errorCapability struct {
	Code               string `json:"code"`
	Description        string `json:"description"`
	Retryable          bool   `json:"retryable"`
	RequiresUserAction bool   `json:"requires_user_action"`
}

func capabilities(opts runOptions) error {
	result := buildCapabilities()
	if opts.structuredJSON() {
		return writeJSONSuccess("capabilities", result)
	}

	fmt.Printf("wadb %s agent contract (schema version %d)\n", result.CLIVersion, result.SchemaVersion)
	fmt.Printf("Structured output: %s; non-interactive mode: %s\n\n", result.StructuredOutputFlag, result.NonInteractiveFlag)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "COMMAND\tMUTATES\tUSER ACTION\tJSON\tDESCRIPTION")
	for _, command := range result.Commands {
		fmt.Fprintf(w, "%s\t%t\t%t\t%t\t%s\n", command.Invocation, command.MutatesState, command.RequiresUserAction, command.SupportsStructuredOutput, command.Description)
	}
	return w.Flush()
}

func buildCapabilities() capabilitiesResult {
	return capabilitiesResult{
		CLIVersion:           version,
		SchemaVersion:        jsonSchemaVersion,
		SchemaFile:           "schemas/wadb-output-v1.schema.json",
		SchemaCommand:        "wadb schema",
		StructuredOutputFlag: "--output json",
		NonInteractiveFlag:   "--non-interactive",
		LegacyJSONFlag:       "--json",
		Commands: []commandCapability{
			{Name: "pair_qr", Invocation: "wadb", Description: "Pair and connect by scanning a terminal QR code.", MutatesState: true, RequiresUserAction: true},
			{Name: "pair", Invocation: "wadb pair <host:port>", Description: "Pair with Android's six-digit code and connect.", MutatesState: true, RequiresUserAction: true, RequiresStdin: true, SupportsStructuredOutput: true},
			{Name: "connect", Invocation: "wadb connect", Description: "Reconnect an already paired wireless device.", MutatesState: true, SupportsStructuredOutput: true, DeviceSelection: "required_when_ambiguous", AllowsAll: true},
			{Name: "devices", Invocation: "wadb devices", Description: "List connected and discovered Android devices.", SupportsStructuredOutput: true, DeviceSelection: "optional_filter"},
			{Name: "disconnect", Invocation: "wadb disconnect", Description: "Disconnect one explicitly selected wireless device or all devices.", MutatesState: true, SupportsStructuredOutput: true, DeviceSelection: "required_or_all", AllowsAll: true},
			{Name: "doctor", Invocation: "wadb doctor", Description: "Inspect adb, interfaces, and mDNS visibility.", SupportsStructuredOutput: true},
			{Name: "capabilities", Invocation: "wadb capabilities", Description: "Describe the agent-facing CLI contract.", SupportsStructuredOutput: true},
			{Name: "schema", Invocation: "wadb schema", Description: "Print the embedded JSON Schema for structured output.", SupportsStructuredOutput: true},
		},
		Errors: knownErrorCapabilities(),
	}
}
