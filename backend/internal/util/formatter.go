package util

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// Row is implemented by types that can be serialized to a string slice
// for table or CSV output.
type Row interface {
	ToRow() []string
}

// Column represents a table column with a header label.
type Column struct {
	Header string
}

// TableFormatter formats tabular data as aligned tabs or CSV.
type TableFormatter struct {
	columns []Column
	format  string
	writer  *tabwriter.Writer
}

// NewTableFormatter creates a TableFormatter for the given columns and format.
// format: "csv" for CSV output, anything else for tabwriter output.
func NewTableFormatter(columns []Column, format string) *TableFormatter {
	return &TableFormatter{
		columns: columns,
		format:  format,
		writer:  tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0),
	}
}

// PrintHeader writes the header row.
func (f *TableFormatter) PrintHeader() {
	headers := make([]string, len(f.columns))
	for i, col := range f.columns {
		headers[i] = col.Header
	}
	if f.format == "csv" {
		w := csv.NewWriter(os.Stdout)
		if err := w.Write(headers); err != nil {
			fmt.Fprintf(os.Stderr, "write csv header: %v\n", err)
			os.Exit(1)
		}
		w.Flush()
		return
	}
	if _, err := fmt.Fprintln(f.writer, strings.Join(headers, "\t")); err != nil {
		fmt.Fprintf(os.Stderr, "write table header: %v\n", err)
		os.Exit(1)
	}
}

// PrintRows writes data rows and flushes for tabwriter.
func (f *TableFormatter) PrintRows(rows [][]string) {
	if f.format == "csv" {
		w := csv.NewWriter(os.Stdout)
		for _, row := range rows {
			if err := w.Write(row); err != nil {
				fmt.Fprintf(os.Stderr, "write csv row: %v\n", err)
				os.Exit(1)
			}
		}
		w.Flush()
		return
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(f.writer, strings.Join(row, "\t")); err != nil {
			fmt.Fprintf(os.Stderr, "write table row: %v\n", err)
			os.Exit(1)
		}
	}
	if err := f.writer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "flush table output: %v\n", err)
		os.Exit(1)
	}
}

// GroupByColumns groups rows by the values at the named columns.
// columnNames is a comma-separated list of column header names.
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
			return nil, nil, fmt.Errorf("column %q not found; available: %s", name, strings.Join(headers, ", "))
		}
	}

	counts := make(map[string]int)
	keys := make(map[string][]string)
	for _, row := range rows {
		vals := make([]string, len(indices))
		for i, idx := range indices {
			vals[i] = row[idx]
		}
		composite := strings.Join(vals, "\x00")
		counts[composite]++
		keys[composite] = vals
	}

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
		outCols = append(outCols, Column{Header: name})
	}
	outCols = append(outCols, Column{Header: "Count"})
	return outCols, outRows, nil
}
