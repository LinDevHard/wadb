package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
)

//go:embed schemas/wadb-output-v1.schema.json
var outputSchemaV1 []byte

func printSchema(opts runOptions) error {
	if opts.structuredJSON() {
		var schema any
		if err := json.Unmarshal(outputSchemaV1, &schema); err != nil {
			return fmt.Errorf("decode embedded JSON Schema: %w", err)
		}
		return writeJSONSuccess("schema", map[string]any{"schema": schema})
	}
	if _, err := os.Stdout.Write(outputSchemaV1); err != nil {
		return fmt.Errorf("write JSON Schema: %w", err)
	}
	if len(outputSchemaV1) == 0 || outputSchemaV1[len(outputSchemaV1)-1] != '\n' {
		fmt.Fprintln(os.Stdout)
	}
	return nil
}
