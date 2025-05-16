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
	assert.Contains(t, err.Error(), "json Marshal error")
}

func TestParser_Array(t *testing.T) {
	// Test array to JSON
	testArray := []int{1, 2, 3, 4, 5}

	// Parse array to JSON bytes
	bytes, err := Parser(testArray)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	// Verify the JSON content is correct
	var result []int
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)
	assert.Equal(t, len(testArray), len(result))
	for i, v := range testArray {
		assert.Equal(t, v, result[i])
	}
}

func TestParser_NestedStructs(t *testing.T) {
	// Test complex nested structs
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		Country string `json:"country"`
	}

	type Person struct {
		Name      string    `json:"name"`
		Age       int       `json:"age"`
		Addresses []Address `json:"addresses"`
	}

	testData := Person{
		Name: "John Doe",
		Age:  30,
		Addresses: []Address{
			{
				Street:  "123 Main St",
				City:    "San Francisco",
				Country: "USA",
			},
			{
				Street:  "456 High St",
				City:    "New York",
				Country: "USA",
			},
		},
	}

	// Parse nested struct to JSON bytes
	bytes, err := Parser(testData)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	// Verify the JSON content is correct
	var result Person
	err = json.Unmarshal(bytes, &result)
	assert.NoError(t, err)
	assert.Equal(t, testData.Name, result.Name)
	assert.Equal(t, testData.Age, result.Age)
	assert.Equal(t, len(testData.Addresses), len(result.Addresses))
	for i, addr := range testData.Addresses {
		assert.Equal(t, addr.Street, result.Addresses[i].Street)
		assert.Equal(t, addr.City, result.Addresses[i].City)
		assert.Equal(t, addr.Country, result.Addresses[i].Country)
	}
}

func TestParser_Primitives(t *testing.T) {
	// Test various primitive types
	testCases := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "hello", `"hello"`},
		{"int", 42, `42`},
		{"float", 3.14, `3.14`},
		{"bool true", true, `true`},
		{"bool false", false, `false`},
		{"null", nil, `null`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse to JSON bytes
			bytes, err := Parser(tc.input)

			// Verify results
			assert.NoError(t, err)
			assert.NotNil(t, bytes)
			assert.Equal(t, tc.expected, string(bytes))
		})
	}
}
