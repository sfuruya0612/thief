package util

import (
	"encoding/json"
	"fmt"
)

// Parser は任意の値を JSON バイト列にシリアライズする。
func Parser(i interface{}) ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("json Marshal error: %v", err)
	}

	return bytes, nil
}
