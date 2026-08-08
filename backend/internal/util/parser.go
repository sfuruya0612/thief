package util

import (
	"encoding/json"
	"fmt"
)

// Parser は任意の値を JSON バイト列にシリアライズする。
//
// 失敗は %w でラップして返す。encoding/json が返す *json.UnsupportedTypeError、
// *json.UnsupportedValueError、*json.MarshalerError に呼び出し側が errors.As で
// 到達できるようにするためである。*json.MarshalerError は自身も Unwrap を持つため、
// MarshalJSON が返した元のエラーまでたどれる。
//
// 文言に json を含めないのは、呼び出し元がいずれも marshal を含む接頭辞を前置しており、
// encoding/json 自身のエラーも json: で始まるためである。どちらとも重複しない語を選ぶ。
func Parser(i interface{}) ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("encode value: %w", err)
	}

	return bytes, nil
}
