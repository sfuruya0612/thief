package util

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTableFormatter(t *testing.T) {
	columns := []Column{
		{Header: "ID", Width: 10},
		{Header: "Name", Width: 20},
	}
	format := "table"

	formatter := NewTableFormatter(columns, format)

	assert.NotNil(t, formatter)
	assert.Equal(t, columns, formatter.columns)
	assert.Equal(t, format, formatter.format)
}

func TestTableFormatter_PrintHeader_Table(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	// Create formatter
	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "table")

	// Call function to test
	formatter.PrintHeader()

	// Get output
	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output
	expected := "ID   \tName      \n"
	assert.Equal(t, expected, output)
}

func TestTableFormatter_PrintRows_Table(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	// Create formatter
	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "table")

	// Test data
	rows := [][]string{
		{"1", "Test 1"},
		{"2", "Test 2"},
	}

	// Call function to test
	formatter.PrintRows(rows)

	// Get output
	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output
	expected := "1    \tTest 1    \n2    \tTest 2    \n"
	assert.Equal(t, expected, output)
}

func TestTableFormatter_PrintHeader_CSV(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	// Create formatter
	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "csv")

	// Call function to test
	formatter.PrintHeader()

	// Get output
	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output - note that CSV adds newline at the end
	expected := "ID,Name\n"
	assert.Equal(t, expected, output)
}

func TestTableFormatter_PrintRows_CSV(t *testing.T) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	// Create formatter
	columns := []Column{
		{Header: "ID", Width: 5},
		{Header: "Name", Width: 10},
	}
	formatter := NewTableFormatter(columns, "csv")

	// Test data
	rows := [][]string{
		{"1", "Test 1"},
		{"2", "Test 2"},
	}

	// Call function to test
	formatter.PrintRows(rows)

	// Get output
	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output
	expected := "1,Test 1\n2,Test 2\n"
	assert.Equal(t, expected, output)
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
	assert.Equal(t, expectedFormat, format)
}
