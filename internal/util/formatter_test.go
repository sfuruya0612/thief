package util

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"text/tabwriter"
)

func TestNewTableFormatter(t *testing.T) {
	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	format := "table"

	formatter := NewTableFormatter(columns, format)

	if formatter == nil {
		t.Fatal("expected non-nil formatter, got nil")
	}
	if len(formatter.columns) != len(columns) {
		t.Errorf("expected %d columns, got %d", len(columns), len(formatter.columns))
	}
	if formatter.format != format {
		t.Errorf("expected format %q, got %q", format, formatter.format)
	}
	if formatter.writer == nil {
		t.Error("expected non-nil tabwriter, got nil")
	}
}

func TestTableFormatter_PrintHeader_And_PrintRows_Table(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	formatter := NewTableFormatter(columns, "table")
	// Point the internal tabwriter at our pipe so we can capture output.
	formatter.writer = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	formatter.PrintHeader()
	rows := [][]string{
		{"1", "Test 1"},
		{"2", "Test 2"},
	}
	formatter.PrintRows(rows)

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// tabwriter with padding=2 aligns columns dynamically.
	// Columns: "ID" (len 2), "Name" (len 6 for "Test 1"/"Test 2") → ID padded to match widths.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d: %q", len(lines), output)
	}

	// Verify header contains both column names.
	if !strings.Contains(lines[0], "ID") || !strings.Contains(lines[0], "Name") {
		t.Errorf("header should contain ID and Name, got %q", lines[0])
	}

	// Verify rows contain data.
	if !strings.Contains(lines[1], "1") || !strings.Contains(lines[1], "Test 1") {
		t.Errorf("row 1 should contain '1' and 'Test 1', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "2") || !strings.Contains(lines[2], "Test 2") {
		t.Errorf("row 2 should contain '2' and 'Test 2', got %q", lines[2])
	}
}

func TestTableFormatter_PrintHeader_CSV(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	formatter := NewTableFormatter(columns, "csv")

	formatter.PrintHeader()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	expected := "ID,Name\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestTableFormatter_PrintRows_CSV(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	formatter := NewTableFormatter(columns, "csv")

	rows := [][]string{
		{"1", "Test 1"},
		{"2", "Test 2"},
	}

	formatter.PrintRows(rows)

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	expected := "1,Test 1\n2,Test 2\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestTableFormatter_PrintHeader_UnexpectedFormat(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	formatter := NewTableFormatter(columns, "custom_format")
	formatter.writer = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	formatter.PrintHeader()
	// Flush via PrintRows with empty rows to trigger output.
	formatter.PrintRows([][]string{})

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "ID") || !strings.Contains(output, "Name") {
		t.Errorf("expected output to contain ID and Name, got %q", output)
	}
}

func TestTableFormatter_PrintRows_EmptyRows(t *testing.T) {
	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	formatter := NewTableFormatter(columns, "table")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	formatter.writer = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	formatter.PrintRows([][]string{})

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("expected empty output, got %q", output)
	}
}

func TestTableFormatter_DynamicColumnWidth(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID"},
		{Header: "Name"},
	}
	formatter := NewTableFormatter(columns, "table")
	formatter.writer = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	formatter.PrintHeader()
	rows := [][]string{
		{"1", "short"},
		{"2", "a very long name value here"},
	}
	formatter.PrintRows(rows)

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), output)
	}

	// All lines should have the same column alignment (the Name column should
	// be wide enough for the longest value).
	// Find the position of the second column in each line — they should match.
	headerNameIdx := strings.Index(lines[0], "Name")
	row1NameIdx := strings.Index(lines[1], "short")
	row2NameIdx := strings.Index(lines[2], "a very long name value here")
	if headerNameIdx != row1NameIdx || headerNameIdx != row2NameIdx {
		t.Errorf("columns not aligned: header=%d, row1=%d, row2=%d\noutput:\n%s",
			headerNameIdx, row1NameIdx, row2NameIdx, output)
	}
}

func TestTableFormatter_CSV_WriteError(t *testing.T) {
	// Test error handling for CSV writer would be beneficial, but it's difficult
	// to test directly without modifying the original code to make it more testable.
	t.Skip("Skipping CSV writer error test as it requires code refactoring")
}

func TestGroupByColumns_Single(t *testing.T) {
	columns := []Column{
		{Header: "Name"},
		{Header: "State"},
		{Header: "Type"},
	}
	rows := [][]string{
		{"a", "running", "t3.micro"},
		{"b", "stopped", "t3.small"},
		{"c", "running", "t3.micro"},
		{"d", "running", "t3.small"},
	}

	outCols, outRows, err := GroupByColumns(columns, rows, "State")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outCols) != 2 {
		t.Fatalf("expected 2 output columns, got %d", len(outCols))
	}
	if outCols[0].Header != "State" {
		t.Errorf("expected first column 'State', got %q", outCols[0].Header)
	}
	if outCols[1].Header != "Count" {
		t.Errorf("expected second column 'Count', got %q", outCols[1].Header)
	}
	if len(outRows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(outRows))
	}
	// sorted: running=3, stopped=1
	if outRows[0][0] != "running" || outRows[0][1] != "3" {
		t.Errorf("expected running=3, got %v", outRows[0])
	}
	if outRows[1][0] != "stopped" || outRows[1][1] != "1" {
		t.Errorf("expected stopped=1, got %v", outRows[1])
	}
}

func TestGroupByColumns_Multiple(t *testing.T) {
	columns := []Column{
		{Header: "State"},
		{Header: "Type"},
	}
	rows := [][]string{
		{"running", "t3.micro"},
		{"running", "t3.micro"},
		{"running", "t3.small"},
		{"stopped", "t3.small"},
	}

	outCols, outRows, err := GroupByColumns(columns, rows, "State,Type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outCols) != 3 {
		t.Fatalf("expected 3 output columns, got %d", len(outCols))
	}
	if len(outRows) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(outRows))
	}
	// sorted: running/t3.micro=2, running/t3.small=1, stopped/t3.small=1
	if outRows[0][0] != "running" || outRows[0][1] != "t3.micro" || outRows[0][2] != "2" {
		t.Errorf("unexpected row[0]: %v", outRows[0])
	}
	if outRows[1][0] != "running" || outRows[1][1] != "t3.small" || outRows[1][2] != "1" {
		t.Errorf("unexpected row[1]: %v", outRows[1])
	}
	if outRows[2][0] != "stopped" || outRows[2][1] != "t3.small" || outRows[2][2] != "1" {
		t.Errorf("unexpected row[2]: %v", outRows[2])
	}
}

func TestGroupByColumns_InvalidColumn(t *testing.T) {
	columns := []Column{
		{Header: "State"},
	}
	rows := [][]string{{"running"}}

	_, _, err := GroupByColumns(columns, rows, "NoSuchColumn")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NoSuchColumn") {
		t.Errorf("expected error to mention 'NoSuchColumn', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "State") {
		t.Errorf("expected error to list available column 'State', got %q", err.Error())
	}
}

func TestGroupByColumns_EmptyRows(t *testing.T) {
	columns := []Column{
		{Header: "State"},
	}

	outCols, outRows, err := GroupByColumns(columns, [][]string{}, "State")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outCols) != 2 {
		t.Errorf("expected 2 output columns, got %d", len(outCols))
	}
	if len(outRows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(outRows))
	}
}
