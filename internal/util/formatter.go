package util

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Row is implemented by types that can be serialised to a string slice for
// table or CSV output. Implementing Row enables use of the generic runList
// helper in the cmd package.
type Row interface {
	ToRow() []string
}

// Column represents a table column with a header and its display width.
// Used for formatting tabular output in the CLI.
type Column struct {
	Header string
	Width  int
}

// TableFormatter provides functionality for formatting and printing tabular data.
// It supports multiple output formats including formatted tables and CSV.
type TableFormatter struct {
	columns []Column
	format  string
}

// NewTableFormatter creates a new TableFormatter with the specified columns and output format.
// The format parameter can be "csv" for CSV output or any other value for tabular output.
func NewTableFormatter(columns []Column, format string) *TableFormatter {
	return &TableFormatter{
		columns: columns,
		format:  format,
	}
}

// PrintHeader prints the table headers based on the configured format.
// For CSV format, it writes a CSV header row to stdout.
// For tabular format, it prints a formatted header row with appropriate column widths.
func (f *TableFormatter) PrintHeader() {
	if f.format == "csv" {
		writer := csv.NewWriter(os.Stdout)
		headers := make([]string, len(f.columns))
		for i, col := range f.columns {
			headers[i] = col.Header
		}
		if err := writer.Write(headers); err != nil {
			fmt.Printf("Unable to write CSV header: %v\n", err)
			os.Exit(1)
		}
		writer.Flush()
		return
	}

	format := f.createFormatString()
	headers := make([]interface{}, len(f.columns))
	for i, col := range f.columns {
		headers[i] = col.Header
	}
	fmt.Printf(format, headers...)
}

// PrintRows prints the data rows based on the configured format.
// For CSV format, it writes CSV rows to stdout.
// For tabular format, it prints formatted rows with appropriate column widths.
func (f *TableFormatter) PrintRows(rows [][]string) {
	if f.format == "csv" {
		writer := csv.NewWriter(os.Stdout)
		for _, row := range rows {
			if err := writer.Write(row); err != nil {
				fmt.Printf("Unable to write CSV row: %v\n", err)
				os.Exit(1)
			}
		}
		writer.Flush()
		return
	}

	format := f.createFormatString()
	for _, row := range rows {
		rowValues := make([]interface{}, len(row))
		for i, v := range row {
			rowValues[i] = v
		}
		fmt.Printf(format, rowValues...)
	}
}

// GroupByColumns groups rows by the values at the columns matching columnNames.
// columnNames is a comma-separated string (e.g. "State" or "State,InstanceType").
// It returns the output columns and sorted (key1, key2, ..., count) rows.
// Returns an error listing available columns if any name is not found.
func GroupByColumns(columns []Column, rows [][]string, columnNames string) ([]Column, [][]string, error) {
	names := strings.Split(columnNames, ",")
	indices := make([]int, len(names))
	for i, name := range names {
		name = strings.TrimSpace(name)
		names[i] = name
		found := false
		for j, col := range columns {
			if col.Header == name {
				indices[i] = j
				found = true
				break
			}
		}
		if !found {
			headers := make([]string, len(columns))
			for j, col := range columns {
				headers[j] = col.Header
			}
			return nil, nil, fmt.Errorf("column %q not found. available: %s", name, strings.Join(headers, ", "))
		}
	}

	counts := make(map[string]int)
	keys := make(map[string][]string) // composite key → individual values
	for _, row := range rows {
		vals := make([]string, len(indices))
		for i, idx := range indices {
			vals[i] = row[idx]
		}
		composite := strings.Join(vals, "\x00")
		counts[composite]++
		keys[composite] = vals
	}

	// Sort composite keys for deterministic output.
	composites := make([]string, 0, len(counts))
	for k := range counts {
		composites = append(composites, k)
	}
	sort.Strings(composites)

	outRows := make([][]string, 0, len(composites))
	for _, composite := range composites {
		row := make([]string, len(keys[composite])+1)
		copy(row, keys[composite])
		row[len(keys[composite])] = strconv.Itoa(counts[composite])
		outRows = append(outRows, row)
	}

	outCols := make([]Column, 0, len(names)+1)
	for _, name := range names {
		outCols = append(outCols, Column{Header: name, Width: 30})
	}
	outCols = append(outCols, Column{Header: "Count", Width: 10})

	return outCols, outRows, nil
}

// createFormatString generates a format string for printing tabular data.
// It creates a format string with the appropriate width for each column,
// separated by tabs and ending with a newline.
func (f *TableFormatter) createFormatString() string {
	formats := make([]string, len(f.columns))
	for i, col := range f.columns {
		formats[i] = fmt.Sprintf("%%-%ds", col.Width)
	}
	return strings.Join(formats, "\t") + "\n"
}
