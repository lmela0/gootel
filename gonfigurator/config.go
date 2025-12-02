package gonfigurator

import (
	"encoding/json"
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
	var fetcher DataFetcher
	switch opt := opts.(type) {
	case EnvOption:
		fetcher = EnvFetcher{}
	case JsonOption:
		content, err := os.ReadFile(opt.Path)
		if err != nil {
			return fmt.Errorf("can't read file '%s': %w", opt.Path, err)
		}
		var jsonMap map[string]interface{}
		if err := json.Unmarshal(content, &jsonMap); err != nil {
			return fmt.Errorf("can't unmarshall file '%s': %w", opt.Path, err)
		}
		fetcher = JsonFetcher{Data: jsonMap}
	default:
		return ErrOptNotSupported
	}
	v := reflect.ValueOf(data).Elem()
	if !v.IsValid() || !v.CanSet() {
		return ErrStructNotValid
	}
	return loadStructFields(v, fetcher)
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
					newStruct := reflect.New(fieldValue.Type().Elem())
					fieldValue.Set(newStruct)
				}
				if err := loadStructFields(fieldValue.Elem(), fetcher); err != nil {
					return err
				}
			} else if tagValue != "" {
				if err := setValue(fieldValue, tagValue, fetcher); err != nil {
					// ignore err
				}
			}
		case reflect.Slice, reflect.Array:
			if tagValue != "" {
				if err := loadSliceOrArray(fieldValue, tagValue, fetcher); err != nil {
					// ignore err
				}
			}
		default:
			if tagValue != "" {
				if err := setValue(fieldValue, tagValue, fetcher); err != nil {
					// ignore err
				}
			}
		}
	}
	return nil
}

// loadSliceOrArray get CSV formatted data and split them into a slice field
// returning an error if anything goes wrong during the load
func loadSliceOrArray(fieldValue reflect.Value, tagValue string, fetcher DataFetcher) error {
	sourceString, err := fetcher.GetValue(tagValue, fieldValue.Kind())
	if err != nil {
		return fmt.Errorf("can't get value for key '%s': %w", tagValue, err)
	}
	if sourceString == "" {
		return nil
	}
	elementStrings := strings.Split(sourceString, ",")
	for i, s := range elementStrings {
		elementStrings[i] = strings.TrimSpace(s)
	}
	elementType := fieldValue.Type().Elem()
	var newSlice reflect.Value
	if fieldValue.Kind() == reflect.Slice {
		newSlice = reflect.MakeSlice(fieldValue.Type(), 0, len(elementStrings))
	}
	for _, s := range elementStrings {
		newElement := reflect.New(elementType).Elem()
		if err := setAtomicValue(newElement, s); err != nil {
			return fmt.Errorf("coversion error for value '%s': %w", s, err)
		}
		if fieldValue.Kind() == reflect.Slice {
			newSlice = reflect.Append(newSlice, newElement)
		}
	}
	if fieldValue.Kind() == reflect.Slice {
		fieldValue.Set(newSlice)
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
