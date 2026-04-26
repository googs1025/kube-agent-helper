package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
)

// printOutput renders data in the requested format (table, json, yaml).
// For table format, headers and rows are used.
// For json/yaml, the raw data object is serialized.
func printOutput(format string, headers []string, rows [][]string, data interface{}) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		// Convert through JSON first to get proper field names
		j, err := json.Marshal(data)
		if err != nil {
			return err
		}
		y, err := sigsyaml.JSONToYAML(j)
		if err != nil {
			return err
		}
		fmt.Print(string(y))
		return nil
	default: // table
		if len(headers) == 0 {
			// Single object: use json as fallback
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(data)
		}
		printTable(headers, rows)
		return nil
	}
}

// printTable renders a simple aligned table.
func printTable(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, col := range row {
			if i < len(widths) && len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}

	// Print header
	parts := make([]string, len(headers))
	for i, h := range headers {
		parts[i] = fmt.Sprintf("%-*s", widths[i], h)
	}
	fmt.Println(strings.Join(parts, "  "))

	// Print rows
	for _, row := range rows {
		parts := make([]string, len(headers))
		for i := range headers {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			parts[i] = fmt.Sprintf("%-*s", widths[i], val)
		}
		fmt.Println(strings.Join(parts, "  "))
	}
}
