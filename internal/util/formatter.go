package util

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

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
