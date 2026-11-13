package config

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/skolldire/go-engine/config/internal/paths"
)

func (s *Snapshot) section(path string) (*node, bool) {
	if s == nil || s.root == nil {
		return nil, false
	}
	if path == "" {
		return s.root, true
	}
	if !paths.Valid(path) {
		return nil, false
	}
	current := s.root
	for _, part := range strings.Split(path, ".") {
		next, ok := current.object[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
func (s *Snapshot) Has(section string) bool { _, ok := s.section(section); return ok }
func (s *Snapshot) Decode(section string, target any) error {
	if section != "" && !paths.Valid(section) {
		return &ConfigError{section, "invalid section path"}
	}
	n, exists := s.section(section)
	if !exists {
		return fmt.Errorf("%s: %w", section, ErrSectionMissing)
	}
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return &ConfigError{section, "target must be a non-nil pointer"}
	}
	copy, err := cloneTarget(value.Elem(), section, 1, s.maxDepth, map[reflect.Type]bool{})
	if err != nil {
		return err
	}
	if err := decodeValue(n, copy, section, 1, s.maxDepth); err != nil {
		return err
	}
	value.Elem().Set(copy)
	return nil
}
func typeError(path, origin, reason string) error {
	return &ConfigError{path, reason + " from " + origin}
}
func textType(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}
func cloneTarget(value reflect.Value, path string, depth, maxDepth int, types map[reflect.Type]bool) (reflect.Value, error) {
	if depth > maxDepth {
		return reflect.Value{}, fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	t := value.Type()
	copy := reflect.New(t).Elem()
	if textType(t) && t.Kind() != reflect.Pointer && t.Kind() != reflect.Interface && t.Kind() != reflect.Func && t.Kind() != reflect.Chan {
		copy.Set(value)
		return copy, nil
	}
	switch t.Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		copy.Set(value)
	case reflect.Struct:
		copy.Set(value)
		if textType(t) {
			return copy, nil
		}
		if _, err := fields(t, path, map[reflect.Type]bool{}); err != nil {
			return reflect.Value{}, err
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() || field.Tag.Get("config") == "-" {
				continue
			}
			child, err := cloneTarget(value.Field(i), paths.Join(path, keyName(field)), depth+1, maxDepth, types)
			if err != nil {
				return reflect.Value{}, err
			}
			copy.Field(i).Set(child)
		}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return reflect.Value{}, &ConfigError{path, "only string map keys supported"}
		}
		if err := validateType(t.Elem(), path, depth+1, maxDepth, types); err != nil {
			return reflect.Value{}, err
		}
		if !value.IsNil() {
			copy.Set(reflect.MakeMapWithSize(t, value.Len()))
			iter := value.MapRange()
			for iter.Next() {
				key := iter.Key()
				child, err := cloneTarget(iter.Value(), paths.Join(path, key.String()), depth+1, maxDepth, types)
				if err != nil {
					return reflect.Value{}, err
				}
				copy.SetMapIndex(key, child)
			}
		}
	case reflect.Slice, reflect.Array:
		if err := validateType(t.Elem(), path, depth+1, maxDepth, types); err != nil {
			return reflect.Value{}, err
		}
		if t.Kind() == reflect.Slice && !value.IsNil() {
			copy.Set(reflect.MakeSlice(t, value.Len(), value.Len()))
		}
		for i := 0; i < value.Len(); i++ {
			child, err := cloneTarget(value.Index(i), fmt.Sprintf("%s[%d]", path, i), depth+1, maxDepth, types)
			if err != nil {
				return reflect.Value{}, err
			}
			copy.Index(i).Set(child)
		}
	default:
		return reflect.Value{}, &ConfigError{path, "unsupported target type"}
	}
	return copy, nil
}
func validateType(t reflect.Type, path string, depth, maxDepth int, types map[reflect.Type]bool) error {
	if depth > maxDepth {
		return fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	if textType(t) && t.Kind() != reflect.Pointer && t.Kind() != reflect.Interface && t.Kind() != reflect.Func && t.Kind() != reflect.Chan {
		return nil
	}
	if types[t] {
		return nil
	}
	types[t] = true
	defer delete(types, t)
	switch t.Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		return nil
	case reflect.Struct:
		if textType(t) {
			return nil
		}
		if _, err := fields(t, path, map[reflect.Type]bool{}); err != nil {
			return err
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.IsExported() && f.Tag.Get("config") != "-" {
				if err := validateType(f.Type, paths.Join(path, keyName(f)), depth+1, maxDepth, types); err != nil {
					return err
				}
			}
		}
		return nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return &ConfigError{path, "only string map keys supported"}
		}
		return validateType(t.Elem(), path, depth+1, maxDepth, types)
	case reflect.Slice, reflect.Array:
		return validateType(t.Elem(), path, depth+1, maxDepth, types)
	default:
		return &ConfigError{path, "unsupported target type"}
	}
}
func keyName(field reflect.StructField) string {
	if tag, ok := field.Tag.Lookup("config"); ok {
		return tag
	}
	chars := []rune(field.Name)
	var out strings.Builder
	for i, r := range chars {
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(chars[i-1]) || unicode.IsDigit(chars[i-1]) || (i+1 < len(chars) && unicode.IsLower(chars[i+1]))) {
			out.WriteByte('_')
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}
func fields(t reflect.Type, path string, visiting map[reflect.Type]bool) (map[string][]int, error) {
	if visiting[t] {
		return nil, &ConfigError{path, "recursive embedded type"}
	}
	visiting[t] = true
	defer delete(visiting, t)
	result := map[string][]int{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Tag.Get("config") == "-" {
			continue
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("config") == "" && !textType(f.Type) {
			nested, err := fields(f.Type, path, visiting)
			if err != nil {
				return nil, err
			}
			for key, index := range nested {
				if _, exists := result[key]; exists {
					return nil, &ConfigError{paths.Join(path, key), "duplicate target field name"}
				}
				result[key] = append([]int{i}, index...)
			}
		} else {
			key := keyName(f)
			if _, exists := result[key]; exists {
				return nil, &ConfigError{paths.Join(path, key), "duplicate target field name"}
			}
			result[key] = []int{i}
		}
	}
	return result, nil
}
func decodeValue(n *node, target reflect.Value, path string, depth, maxDepth int) error {
	if depth > maxDepth {
		return fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	fail := func(reason string) error { return typeError(path, n.origin, reason) }
	if n.isNull() {
		return fail("null cannot be decoded")
	}
	if target.Type() == reflect.TypeFor[time.Duration]() {
		value, ok := n.value.(string)
		if !ok || value == "0" {
			return fail("duration requires a string with unit")
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fail("invalid duration")
		}
		target.SetInt(int64(duration))
		return nil
	}
	if textType(target.Type()) {
		value, ok := n.value.(string)
		if !ok {
			return fail("text unmarshaler requires string")
		}
		fresh := reflect.New(target.Type())
		if err := fresh.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(value)); err != nil {
			return fail("invalid text value")
		}
		target.Set(fresh.Elem())
		return nil
	}
	switch target.Kind() {
	case reflect.Struct:
		if n.object == nil {
			return fail("expected object")
		}
		mapping, err := fields(target.Type(), path, map[reflect.Type]bool{})
		if err != nil {
			return err
		}
		for _, key := range sortedKeys(n.object) {
			index, ok := mapping[key]
			if !ok {
				return typeError(paths.Join(path, key), n.object[key].origin, "unknown field")
			}
			if err := decodeValue(n.object[key], target.FieldByIndex(index), paths.Join(path, key), depth+1, maxDepth); err != nil {
				return err
			}
		}
	case reflect.Map:
		if n.object == nil {
			return fail("expected object")
		}
		if target.IsNil() {
			target.Set(reflect.MakeMap(target.Type()))
		}
		for _, key := range sortedKeys(n.object) {
			k := reflect.ValueOf(key).Convert(target.Type().Key())
			item := reflect.New(target.Type().Elem()).Elem()
			if old := target.MapIndex(k); old.IsValid() {
				item.Set(old)
			}
			if err := decodeValue(n.object[key], item, paths.Join(path, key), depth+1, maxDepth); err != nil {
				return err
			}
			target.SetMapIndex(k, item)
		}
	case reflect.Slice, reflect.Array:
		if n.list == nil {
			return fail("expected list")
		}
		if target.Kind() == reflect.Array && target.Len() != len(n.list) {
			return fail("array length mismatch")
		}
		if target.Kind() == reflect.Slice {
			target.Set(reflect.MakeSlice(target.Type(), len(n.list), len(n.list)))
		}
		for i, item := range n.list {
			if err := decodeValue(item, target.Index(i), fmt.Sprintf("%s[%d]", path, i), depth+1, maxDepth); err != nil {
				return err
			}
		}
	case reflect.String:
		value, ok := n.value.(string)
		if !ok {
			return fail("expected string")
		}
		target.SetString(value)
	case reflect.Bool:
		if value, ok := n.value.(bool); ok {
			target.SetBool(value)
			return nil
		}
		if value, ok := n.value.(string); ok && n.textual && (value == "true" || value == "false") {
			target.SetBool(value == "true")
			return nil
		}
		return fail("expected boolean")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		number, ok := n.value.(json.Number)
		if !ok {
			value, text := n.value.(string)
			if !text || !n.textual || !validNumber(value) {
				return fail("expected number")
			}
			number = json.Number(value)
		}
		raw := string(number)
		if target.Kind() >= reflect.Int && target.Kind() <= reflect.Int64 {
			if strings.ContainsAny(raw, ".eE") {
				return fail("expected decimal integer")
			}
			value, err := strconv.ParseInt(raw, 10, target.Type().Bits())
			if err != nil {
				return fail("integer overflow")
			}
			target.SetInt(value)
		} else if target.Kind() >= reflect.Uint && target.Kind() <= reflect.Uintptr {
			if strings.ContainsAny(raw, "-.eE") {
				return fail("expected unsigned decimal integer")
			}
			value, err := strconv.ParseUint(raw, 10, target.Type().Bits())
			if err != nil {
				return fail("integer overflow")
			}
			target.SetUint(value)
		} else {
			value, err := strconv.ParseFloat(raw, target.Type().Bits())
			if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
				return fail("float overflow")
			}
			if target.Kind() == reflect.Float32 {
				actual := new(big.Rat).SetFloat64(value).FloatString(149)
				sourceDigits, sourceExponent, sourceSign, sourceOK := decimalParts(raw)
				actualDigits, actualExponent, actualSign, _ := decimalParts(actual)
				if !sourceOK || sourceDigits != actualDigits || sourceExponent != actualExponent || sourceSign != actualSign {
					return fail("float32 loses precision")
				}
			}
			target.SetFloat(value)
		}
	default:
		return fail("unsupported target type")
	}
	return nil
}

func decimalParts(value string) (string, int64, bool, bool) {
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(value), "e")
	fraction := 0
	if point := strings.IndexByte(mantissa, '.'); point >= 0 {
		fraction = len(mantissa) - point - 1
		mantissa = mantissa[:point] + mantissa[point+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return "0", 0, false, true
	}
	exponent := int64(0)
	if hasExponent {
		parsed, err := strconv.ParseInt(exponentText, 10, 64)
		if err != nil {
			return "", 0, false, false
		}
		exponent = parsed
	}
	trimmed := strings.TrimRight(digits, "0")
	adjustment := int64(len(digits)-len(trimmed)) - int64(fraction)
	if adjustment > 0 && exponent > math.MaxInt64-adjustment || adjustment < 0 && exponent < math.MinInt64-adjustment {
		return "", 0, false, false
	}
	return trimmed, exponent + adjustment, negative, true
}
