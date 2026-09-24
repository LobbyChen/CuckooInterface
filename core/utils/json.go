package utils

import (
	"encoding/json"
	"os"
)

func LoadJson[T any](path string) (T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		var zero T
		return zero, err
	}
	var obj T
	if err := json.Unmarshal(data, &obj); err != nil {
		var zero T
		return zero, err
	}
	return obj, nil
}
