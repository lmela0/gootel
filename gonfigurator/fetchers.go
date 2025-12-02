package gonfigurator

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// DataFetcher abstract data accessor used to retrieve data to be mapped
type DataFetcher interface {
	GetValue(key string, targetKind reflect.Kind) (string, error)
}

// EnvFetcher retrieve data from os environment
type EnvFetcher struct{}

func (f EnvFetcher) GetValue(key string, targetKind reflect.Kind) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		found := false
		for _, envVar := range os.Environ() {
			if len(envVar) > len(key) && envVar[:len(key)] == key && envVar[len(key)] == '=' {
				found = true
				value = envVar[len(key):]
				break
			}
		}
		if !found {
			return "", errors.New("key not found")
		}
	}
	return value, nil
}

// JsonFetcher retrieve data from its internal map loaded from a JSON file
type JsonFetcher struct {
	Data map[string]any
}

func (f JsonFetcher) GetValue(key string, targetKind reflect.Kind) (string, error) {
	if f.Data == nil {
		return "", errors.New("JSON data not loaded")
	}
	keys := strings.Split(key, ".")
	currentData := f.Data

	var value any
	for i, k := range keys {
		if i == len(keys)-1 {
			val, ok := currentData[k]
			if !ok {
				return "", fmt.Errorf("key not found '%s'", key)
			}
			value = val
		} else {
			nextData, ok := currentData[k].(map[string]any)
			if !ok {
				return "", fmt.Errorf("path '%s' not a JSON object", k)
			}
			currentData = nextData
		}
	}
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		if float64(int64(v)) == v {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}
