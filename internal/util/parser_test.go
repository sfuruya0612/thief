package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {
	// Test struct to JSON
	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	testData := TestStruct{
		Name:  "test",
		Value: 123,
	}

	// Parse struct to JSON bytes
	bytes, err := Parser(testData)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	// Verify the JSON content is correct
	var result TestStruct
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)
	assert.Equal(t, testData.Name, result.Name)
	assert.Equal(t, testData.Value, result.Value)
}

func TestParser_Map(t *testing.T) {
	// Test map to JSON
	testMap := map[string]interface{}{
		"name":    "test",
		"value":   123.45,
		"enabled": true,
	}

	// Parse map to JSON bytes
	bytes, err := Parser(testMap)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	// Verify the JSON content is correct
	var result map[string]interface{}
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)
	assert.Equal(t, testMap["name"], result["name"])
	assert.Equal(t, testMap["value"], result["value"])
	assert.Equal(t, testMap["enabled"], result["enabled"])
}

func TestParser_Invalid(t *testing.T) {
	// Test with a value that can't be marshaled to JSON
	// Create a circular reference which will cause json.Marshal to fail
	m1 := make(map[string]interface{})
	m2 := make(map[string]interface{})
	m1["child"] = m2
	m2["parent"] = m1

	// Attempt to parse invalid data
	_, err := Parser(m1)

	// Verify error is returned
	assert.Error(t, err)
}
