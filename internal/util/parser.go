package util

import (
	"encoding/json"
	"fmt"
)

func Parser(i interface{}) ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("json Marshal error: %v", err)
	}

	return bytes, nil
}
