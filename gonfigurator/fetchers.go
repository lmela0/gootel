package gonfigurator

import (
	"encoding/json"
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
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrKeyNotFound, key)
	}
	return value, nil
}

// JsonFetcher retrieve data from its internal map loaded from a JSON file
type JsonFetcher struct {
	Data map[string]any
}

func (f JsonFetcher) GetValue(key string, targetKind reflect.Kind) (string, error) {
	if f.Data == nil {
		return "", fmt.Errorf("JSON data not loaded")
	}

	var current any = f.Data
	for _, k := range strings.Split(key, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("path '%s' is not a JSON object", k)
		}
		value, ok := object[k]
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrKeyNotFound, key)
		}
		current = value
	}

	if current == nil {
		return "", nil
	}
	switch v := current.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case float64:
		if float64(int64(v)) == v {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case []any:
		values := make([]string, len(v))
		for i, item := range v {
			values[i] = fmt.Sprint(item)
		}
		return strings.Join(values, ","), nil
	default:
		return "", fmt.Errorf("unsupported JSON value type %T for key '%s'", current, key)
	}
}
