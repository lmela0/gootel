package gonfigurator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// LoadConfig[T any] main function
// take a pointer to generic type T (*T) to alter the struct
// returning an error if anything goes wrong during the load
func LoadConfig[T any](data *T, opts Options) error {
	fetcher, err := newFetcher(opts)
	if err != nil {
		return err
	}
	if data == nil {
		return ErrStructNotValid
	}
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return ErrStructNotValid
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct || !v.CanSet() {
		return ErrStructNotValid
	}
	return loadStructFields(v, fetcher)
}

// newFetcher builds the fetcher matching the provided option
func newFetcher(opts Options) (DataFetcher, error) {
	switch opt := opts.(type) {
	case EnvOption:
		return EnvFetcher{}, nil
	case *EnvOption:
		if opt == nil {
			return nil, ErrOptNotSupported
		}
		return EnvFetcher{}, nil
	case JsonOption:
		return newJSONFetcher(opt.Path)
	case *JsonOption:
		if opt == nil {
			return nil, ErrOptNotSupported
		}
		return newJSONFetcher(opt.Path)
	default:
		return nil, ErrOptNotSupported
	}
}

// newJSONFetcher reads and decodes a JSON file preserving number precision
func newJSONFetcher(path string) (DataFetcher, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't read file '%s': %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var jsonMap map[string]any
	if err := decoder.Decode(&jsonMap); err != nil {
		return nil, fmt.Errorf("can't unmarshall file '%s': %w", path, err)
	}
	return JsonFetcher{Data: jsonMap}, nil
}

// loadStructFields recursively load fields in data structure
// returning an error if anything goes wrong during the load
func loadStructFields(v reflect.Value, fetcher DataFetcher) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)
		fieldValue := v.Field(i)
		if !fieldValue.CanSet() {
			continue
		}
		tagValue := fieldType.Tag.Get("map")
		switch fieldValue.Kind() {
		case reflect.Struct:
			if err := loadStructFields(fieldValue, fetcher); err != nil {
				return err
			}
		case reflect.Ptr:
			if fieldValue.Type().Elem().Kind() == reflect.Struct {
				if fieldValue.IsNil() {
					fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
				}
				if err := loadStructFields(fieldValue.Elem(), fetcher); err != nil {
					return err
				}
			} else if tagValue != "" {
				if err := setValue(fieldValue, tagValue, fetcher); err != nil && !errors.Is(err, ErrKeyNotFound) {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			if tagValue != "" {
				if err := loadSliceOrArray(fieldValue, tagValue, fetcher); err != nil && !errors.Is(err, ErrKeyNotFound) {
					return err
				}
			}
		default:
			if tagValue != "" {
				if err := setValue(fieldValue, tagValue, fetcher); err != nil && !errors.Is(err, ErrKeyNotFound) {
					return err
				}
			}
		}
	}
	return nil
}

// loadSliceOrArray get CSV formatted data and split them into a slice or array field
// returning an error if anything goes wrong during the load
func loadSliceOrArray(fieldValue reflect.Value, tagValue string, fetcher DataFetcher) error {
	sourceString, err := fetcher.GetValue(tagValue, fieldValue.Kind())
	if err != nil {
		return fmt.Errorf("can't get value for key '%s': %w", tagValue, err)
	}
	if strings.TrimSpace(sourceString) == "" {
		return nil
	}
	elementStrings := strings.Split(sourceString, ",")
	for i, s := range elementStrings {
		elementStrings[i] = strings.TrimSpace(s)
	}

	switch fieldValue.Kind() {
	case reflect.Slice:
		elementType := fieldValue.Type().Elem()
		newSlice := reflect.MakeSlice(fieldValue.Type(), 0, len(elementStrings))
		for _, s := range elementStrings {
			newElement := reflect.New(elementType).Elem()
			if err := setElementValue(newElement, s); err != nil {
				return fmt.Errorf("conversion error for value '%s': %w", s, err)
			}
			newSlice = reflect.Append(newSlice, newElement)
		}
		fieldValue.Set(newSlice)
	case reflect.Array:
		if len(elementStrings) > fieldValue.Len() {
			return fmt.Errorf(
				"too many values for array key '%s': got %d, max %d",
				tagValue, len(elementStrings), fieldValue.Len(),
			)
		}
		for i, s := range elementStrings {
			if err := setElementValue(fieldValue.Index(i), s); err != nil {
				return fmt.Errorf("conversion error for value '%s': %w", s, err)
			}
		}
	default:
		return fmt.Errorf("field type %s not supported", fieldValue.Kind())
	}
	return nil
}

// setValue use the provided data fetcher to retrieve the data and set it to the target field
// returning an error if anything goes wrong during the load
func setValue(fieldValue reflect.Value, tagValue string, fetcher DataFetcher) error {
	sourceValue, err := fetcher.GetValue(tagValue, fieldValue.Kind())
	if err != nil {
		return fmt.Errorf("can't get key '%s': %w", tagValue, err)
	}
	if sourceValue == "" {
		return nil
	}
	return setElementValue(fieldValue, sourceValue)
}

// setElementValue sets a value, allocating pointer fields when needed
func setElementValue(fieldValue reflect.Value, sourceValue string) error {
	if fieldValue.Kind() == reflect.Ptr {
		newValue := reflect.New(fieldValue.Type().Elem())
		if err := setAtomicValue(newValue.Elem(), sourceValue); err != nil {
			return err
		}
		fieldValue.Set(newValue)
		return nil
	}
	return setAtomicValue(fieldValue, sourceValue)
}

// setAtomicValue effectively set the value in the field, parsing string to destination data type
// returning an error if anything goes wrong during the load
func setAtomicValue(v reflect.Value, s string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue, err := strconv.ParseInt(s, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("can't convert '%s' to int: %w", s, err)
		}
		v.SetInt(intValue)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintValue, err := strconv.ParseUint(s, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("can't convert '%s' to uint: %w", s, err)
		}
		v.SetUint(uintValue)
	case reflect.Bool:
		boolValue, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("can't convert '%s' to bool: %w", s, err)
		}
		v.SetBool(boolValue)
	case reflect.Float32, reflect.Float64:
		floatValue, err := strconv.ParseFloat(s, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("can't convert '%s' to float: %w", s, err)
		}
		v.SetFloat(floatValue)
	default:
		return fmt.Errorf("field type %s not supported", v.Kind())
	}
	return nil
}
