package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Loader loads configuration into a target struct.
// Priority: environment variables > struct tag defaults.
type Loader struct {
	prefix string // env var prefix, e.g. "AUTH_SERVICE_"
}

// NewLoader creates a config loader. prefix is used for env var matching.
func NewLoader(prefix string) *Loader {
	return &Loader{prefix: prefix}
}

// Load loads config into target (must be a pointer to struct).
// Supports struct tags: `env:"KEY"` for env var name, `default:"val"` for default value.
func (l *Loader) Load(target interface{}) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: target must be a pointer to struct")
	}
	return l.loadStruct(v.Elem())
}

func (l *Loader) loadStruct(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		if fieldVal.Kind() == reflect.Struct && field.Anonymous {
			if err := l.loadStruct(fieldVal); err != nil {
				return err
			}
			continue
		}

		if fieldVal.Kind() == reflect.Struct {
			if err := l.loadStruct(fieldVal); err != nil {
				return err
			}
			continue
		}

		envKey := field.Tag.Get("env")
		defaultVal := field.Tag.Get("default")

		var val string
		if envKey != "" {
			if l.prefix != "" {
				if v, ok := os.LookupEnv(l.prefix + envKey); ok {
					val = v
				}
			}
			if val == "" {
				if v, ok := os.LookupEnv(envKey); ok {
					val = v
				}
			}
		}

		if val == "" {
			val = defaultVal
		}

		if val == "" {
			continue
		}

		if err := setField(fieldVal, val); err != nil {
			return fmt.Errorf("config: field %s: %w", field.Name, err)
		}
	}
	return nil
}

func setField(f reflect.Value, val string) error {
	switch f.Kind() {
	case reflect.String:
		f.SetString(val)
	case reflect.Int:
		n, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		f.SetInt(int64(n))
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		f.SetBool(b)
	case reflect.Slice:
		if f.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(val, ",")
			for i, p := range parts {
				parts[i] = strings.TrimSpace(p)
			}
			filtered := parts[:0]
			for _, p := range parts {
				if p != "" {
					filtered = append(filtered, p)
				}
			}
			f.Set(reflect.ValueOf(filtered))
		}
	default:
		return fmt.Errorf("unsupported type %s", f.Kind())
	}
	return nil
}

// MustLoad is like Load but panics on error.
func (l *Loader) MustLoad(target interface{}) {
	if err := l.Load(target); err != nil {
		panic(err)
	}
}
