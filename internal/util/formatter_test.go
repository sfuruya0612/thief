package util

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestNewTableFormatter(t *testing.T) {
	columns := []Column{
		{Header: "ID", Width: 10},
		{Header: "Name", Width: 20},
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
}

func TestTableFormatter_PrintHeader_Table(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "table")

	formatter.PrintHeader()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	expected := "ID   \tName      \n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestTableFormatter_PrintRows_Table(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "table")

	rows := [][]string{
		{"1", "Test 1"},
		{"2", "Test 2"},
	}

	formatter.PrintRows(rows)

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	expected := "1    \tTest 1    \n2    \tTest 2    \n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
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
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
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
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
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

func TestTableFormatter_CreateFormatString(t *testing.T) {
	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
		{Header: "Description", Width: 20},
	}
	formatter := NewTableFormatter(columns, "table")

	format := formatter.createFormatString()

	expectedFormat := "%-5s\t%-10s\t%-20s\n"
	if format != expectedFormat {
		t.Errorf("expected format %q, got %q", expectedFormat, format)
	}
}

func TestTableFormatter_CreateFormatString_EmptyColumns(t *testing.T) {
	columns := []Column{}
	formatter := NewTableFormatter(columns, "table")

	format := formatter.createFormatString()

	expectedFormat := "\n"
	if format != expectedFormat {
		t.Errorf("expected format %q, got %q", expectedFormat, format)
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
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "custom_format")

	formatter.PrintHeader()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	expected := "ID   \tName      \n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestTableFormatter_PrintRows_EmptyRows(t *testing.T) {
	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "table")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	formatter.PrintRows([][]string{})

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("expected empty output, got %q", output)
	}
}

func TestTableFormatter_CSV_WriteError(t *testing.T) {
	// Test error handling for CSV writer would be beneficial, but it's difficult
	// to test directly without modifying the original code to make it more testable.
	t.Skip("Skipping CSV writer error test as it requires code refactoring")
}

func TestGroupByColumns_Single(t *testing.T) {
	columns := []Column{
		{Header: "Name", Width: 20},
		{Header: "State", Width: 10},
		{Header: "Type", Width: 10},
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
		{Header: "State", Width: 10},
		{Header: "Type", Width: 10},
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
		{Header: "State", Width: 10},
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
		{Header: "State", Width: 10},
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
