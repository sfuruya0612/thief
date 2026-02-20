package util

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParser(t *testing.T) {
	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	testData := TestStruct{
		Name:  "test",
		Value: 123,
	}

	b, err := Parser(testData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil bytes, got nil")
	}

	var result TestStruct
	err = json.Unmarshal(b, &result)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if result.Name != testData.Name {
		t.Errorf("expected Name %q, got %q", testData.Name, result.Name)
	}
	if result.Value != testData.Value {
		t.Errorf("expected Value %d, got %d", testData.Value, result.Value)
	}
}

func TestParser_Map(t *testing.T) {
	testMap := map[string]interface{}{
		"name":    "test",
		"value":   123.45,
		"enabled": true,
	}

	b, err := Parser(testMap)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil bytes, got nil")
	}

	var result map[string]interface{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if result["name"] != testMap["name"] {
		t.Errorf("expected name %v, got %v", testMap["name"], result["name"])
	}
	if result["value"] != testMap["value"] {
		t.Errorf("expected value %v, got %v", testMap["value"], result["value"])
	}
	if result["enabled"] != testMap["enabled"] {
		t.Errorf("expected enabled %v, got %v", testMap["enabled"], result["enabled"])
	}
}

func TestParser_Invalid(t *testing.T) {
	// Create a circular reference which will cause json.Marshal to fail
	m1 := make(map[string]interface{})
	m2 := make(map[string]interface{})
	m1["child"] = m2
	m2["parent"] = m1

	_, err := Parser(m1)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "json Marshal error") {
		t.Errorf("expected error to contain 'json Marshal error', got %q", err.Error())
	}
}

func TestParser_Array(t *testing.T) {
	testArray := []int{1, 2, 3, 4, 5}

	b, err := Parser(testArray)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil bytes, got nil")
	}

	var result []int
	err = json.Unmarshal(b, &result)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if len(result) != len(testArray) {
		t.Fatalf("expected length %d, got %d", len(testArray), len(result))
	}
	for i, v := range testArray {
		if result[i] != v {
			t.Errorf("expected result[%d] = %d, got %d", i, v, result[i])
		}
	}
}

func TestParser_NestedStructs(t *testing.T) {
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

	b, err := Parser(testData)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil bytes, got nil")
	}

	var result Person
	err = json.Unmarshal(b, &result)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if result.Name != testData.Name {
		t.Errorf("expected Name %q, got %q", testData.Name, result.Name)
	}
	if result.Age != testData.Age {
		t.Errorf("expected Age %d, got %d", testData.Age, result.Age)
	}
	if len(result.Addresses) != len(testData.Addresses) {
		t.Fatalf("expected %d addresses, got %d", len(testData.Addresses), len(result.Addresses))
	}
	for i, addr := range testData.Addresses {
		if result.Addresses[i].Street != addr.Street {
			t.Errorf("expected Addresses[%d].Street %q, got %q", i, addr.Street, result.Addresses[i].Street)
		}
		if result.Addresses[i].City != addr.City {
			t.Errorf("expected Addresses[%d].City %q, got %q", i, addr.City, result.Addresses[i].City)
		}
		if result.Addresses[i].Country != addr.Country {
			t.Errorf("expected Addresses[%d].Country %q, got %q", i, addr.Country, result.Addresses[i].Country)
		}
	}
}

func TestParser_Primitives(t *testing.T) {
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
			b, err := Parser(tc.input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b == nil {
				t.Fatal("expected non-nil bytes, got nil")
			}
			if string(b) != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, string(b))
			}
		})
	}
}
