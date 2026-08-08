package util

import (
	"encoding/json"
	"errors"
	"fmt"
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

// errFailingMarshaler は failingMarshaler が返すエラー。ラップの連鎖の終端として
// 同一性を確かめられるようにしてある。
var errFailingMarshaler = errors.New("failing marshaler")

// failingMarshaler は MarshalJSON が必ず失敗する型。encoding/json はこれを
// *json.MarshalerError で包んで返す。
type failingMarshaler struct{}

func (failingMarshaler) MarshalJSON() ([]byte, error) { return nil, errFailingMarshaler }

// TestParser_Invalid は encoding/json の失敗をラップしたまま返すことを検証する。
//
// errors.As での到達を見るのが要点である。%v でラップすると Unwrap の連鎖が切れ、
// 呼び出し側は json.Marshal が返した型を取り出せなくなる。文言の部分一致だけでは
// %w から %v への差し戻しを検出できないため、両方を見る。
//
// 入力は json.Marshal が返す 3 つの型をそれぞれ引き出すものを選んでいる。循環参照は
// *json.UnsupportedValueError、チャネルは *json.UnsupportedTypeError、MarshalJSON の
// 失敗は *json.MarshalerError になる。
//
// *json.MarshalerError は自身も Unwrap を持つため、この行だけは Parser の %w と
// MarshalerError の Unwrap を合わせた 2 段の連鎖を errors.Is で検証できる。
func TestParser_Invalid(t *testing.T) {
	cyclic := func() interface{} {
		m1 := make(map[string]interface{})
		m2 := make(map[string]interface{})
		m1["child"] = m2
		m2["parent"] = m1
		return m1
	}

	tests := []struct {
		name    string
		input   interface{}
		wantAs  func(error) bool
		wantErr string
		// wantIs は 2 段以上たどれる入力でのみ指定する。nil なら検証しない。
		wantIs error
	}{
		{
			name:    "cycle",
			input:   cyclic(),
			wantAs:  func(err error) bool { return errors.As(err, new(*json.UnsupportedValueError)) },
			wantErr: "*json.UnsupportedValueError",
		},
		{
			name:    "unsupported type",
			input:   make(chan int),
			wantAs:  func(err error) bool { return errors.As(err, new(*json.UnsupportedTypeError)) },
			wantErr: "*json.UnsupportedTypeError",
		},
		{
			name:    "marshaler error",
			input:   failingMarshaler{},
			wantAs:  func(err error) bool { return errors.As(err, new(*json.MarshalerError)) },
			wantErr: "*json.MarshalerError",
			wantIs:  errFailingMarshaler,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Parser(tt.input)

			if b != nil {
				t.Errorf("bytes = %q, want nil on error", b)
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantAs(err) {
				t.Errorf("errors.As(err, %s) = false, want true; got %v (%T)", tt.wantErr, err, err)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("errors.Is(err, %v) = false, want true; got %v", tt.wantIs, err)
			}

			// 文言は「動詞 + 対象」の形式に揃えてある。旧文言の json Marshal error に
			// 戻すとここで落ちる。error という語を含まないことも押さえる。
			if !strings.HasPrefix(err.Error(), "encode value: ") {
				t.Errorf("error = %q, want it to start with %q", err.Error(), "encode value: ")
			}
			if strings.Contains(err.Error(), "encode value: error") {
				t.Errorf("error = %q, want no redundant %q in the wrapper", err.Error(), "error")
			}
		})
	}
}

// TestParser_WrapperDoesNotRepeatCallerPrefix は Parser の文言が呼び出し元の接頭辞と
// 重複しないことを固定する。
//
// util.Parser の呼び出し元はいずれも marshal を含む接頭辞を前置する。Parser 側も
// marshal を名乗ると連結された文言に同じ動詞が 2 度出る。encoding/json 自身のエラーが
// json: で始まるため、json も同じ理由で避けている。
//
// 接頭辞は internal/cli の実際の呼び出し経路 3 つをそのまま並べている。1 つだけを
// 見ると、他の経路で重複が生じても気付けない。
func TestParser_WrapperDoesNotRepeatCallerPrefix(t *testing.T) {
	// 接頭辞は本番の呼び出し元から写したものである。向こうを変えたらここも変える。
	tests := []struct {
		name   string
		prefix string
	}{
		// sessionManagerSessionJSON (session_plugin.go) は接頭辞を付けずに返し、
		// その呼び出し元 (ec2.go / ecs.go) が marshal session を前置する。
		{name: "marshal session", prefix: "marshal session"},
		// ec2.go の StartSession 入力の組み立て。
		{name: "marshal start session input", prefix: "marshal start session input"},
		// ecs.go の Target の組み立て。
		{name: "marshal target", prefix: "marshal target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parser(make(chan int))
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// 呼び出し元と同じ形に連結して、実際に利用者が見る文言で確かめる。
			wrapped := fmt.Errorf("%s: %w", tt.prefix, err)
			got := wrapped.Error()

			if strings.Count(got, "marshal") != 1 {
				t.Errorf("error = %q, want %q to appear exactly once", got, "marshal")
			}
			if strings.Contains(got, "json: json:") {
				t.Errorf("error = %q, want no adjacent repetition of %q", got, "json:")
			}
			if !errors.As(wrapped, new(*json.UnsupportedTypeError)) {
				t.Errorf("errors.As through the caller prefix = false, want true; got %v", wrapped)
			}
		})
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
