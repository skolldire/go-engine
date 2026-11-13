package config

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	ms "github.com/go-viper/mapstructure/v2"
	"github.com/skolldire/go-engine/config/internal/paths"
	"github.com/skolldire/go-engine/config/internal/textvalue"
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
	plan := decodePlan{names: map[string]string{}, origins: map[string]string{}, maxDepth: s.maxDepth}
	raw, err := plan.prepare(n, copy, section, section, 1)
	if err != nil {
		return err
	}
	decoder, err := strictDecoder(copy, section)
	if err != nil {
		return &ConfigError{section, "invalid decoder configuration"}
	}
	if err := decoder.Decode(raw); err != nil {
		name := section
		var decodeErr *ms.DecodeError
		if errors.As(err, &decodeErr) {
			name = decodeErr.Name()
		}
		field, ok := plan.names[name]
		if !ok {
			field = section
		}
		origin, ok := plan.origins[field]
		if !ok {
			origin = n.origin
		}
		reason := "invalid field type or value"
		var configErr *ConfigError
		if errors.As(err, &configErr) {
			reason = configErr.Reason
		}
		return &ConfigError{field, reason + " from " + origin}
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
func matchName(key, name string) bool {
	return strings.EqualFold(key, name) || strings.EqualFold(strings.ReplaceAll(key, "_", ""), strings.ToLower(name))
}
func keyName(field reflect.StructField) string {
	name := strings.Split(field.Tag.Get("mapstructure"), ",")[0]
	if name == "" {
		return field.Name
	}
	return name
}
func fields(t reflect.Type, path string, visiting map[reflect.Type]bool) (map[string][]int, error) {
	if visiting[t] {
		return nil, &ConfigError{path, "recursive embedded type"}
	}
	visiting[t] = true
	defer delete(visiting, t)
	result := map[string][]int{}
	add := func(key string, index []int) error {
		for existing := range result {
			if matchName(key, existing) || matchName(existing, key) {
				return &ConfigError{paths.Join(path, key), "ambiguous target field names"}
			}
		}
		result[key] = index
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() || keyName(field) == "-" {
			continue
		}
		parts := strings.Split(field.Tag.Get("mapstructure"), ",")
		squash := false
		for _, option := range parts[1:] {
			if option == "squash" {
				squash = true
			}
		}
		if squash {
			if field.Type.Kind() != reflect.Struct {
				return nil, &ConfigError{paths.Join(path, keyName(field)), "squash requires a struct"}
			}
			nested, err := fields(field.Type, path, visiting)
			if err != nil {
				return nil, err
			}
			for _, key := range sortedKeys(nested) {
				if err := add(key, append([]int{i}, nested[key]...)); err != nil {
					return nil, err
				}
			}
		} else if err := add(keyName(field), []int{i}); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type decodePlan struct {
	names    map[string]string
	origins  map[string]string
	maxDepth int
}

func (p *decodePlan) prepare(n *node, target reflect.Value, path, name string, depth int) (any, error) {
	if depth > p.maxDepth {
		return nil, fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	p.names[name] = path
	p.origins[path] = n.origin
	if n.isNull() {
		return nil, typeError(path, n.origin, "null cannot be decoded")
	}
	if target.Type() == reflect.TypeFor[time.Duration]() {
		return inputValue(n), nil
	}
	if textType(target.Type()) {
		target.Set(reflect.Zero(target.Type()))
		return inputValue(n), nil
	}
	switch target.Kind() {
	case reflect.Struct:
		if n.object == nil {
			return inputValue(n), nil
		}
		mapping, err := fields(target.Type(), path, map[reflect.Type]bool{})
		if err != nil {
			return nil, err
		}
		result := map[string]any{}
		used := map[string]bool{}
		for _, key := range sortedKeys(n.object) {
			matched := ""
			for _, candidate := range sortedKeys(mapping) {
				if matchName(key, candidate) {
					matched = candidate
					break
				}
			}
			childPath := paths.Join(path, key)
			if matched == "" {
				return nil, typeError(childPath, n.object[key].origin, "unknown field")
			}
			if used[matched] {
				return nil, typeError(childPath, n.object[key].origin, "ambiguous input field")
			}
			used[matched] = true
			child, err := p.prepare(n.object[key], target.FieldByIndex(mapping[matched]), childPath, paths.Join(name, matched), depth+1)
			if err != nil {
				return nil, err
			}
			result[matched] = child
		}
		return result, nil
	case reflect.Map:
		if n.object == nil {
			return inputValue(n), nil
		}
		if target.IsNil() {
			target.Set(reflect.MakeMap(target.Type()))
		}
		result := map[string]any{}
		for _, key := range sortedKeys(n.object) {
			k := reflect.ValueOf(key).Convert(target.Type().Key())
			item := reflect.New(target.Type().Elem()).Elem()
			if old := target.MapIndex(k); old.IsValid() {
				item.Set(old)
			}
			child, err := p.prepare(n.object[key], item, paths.Join(path, key), name+"["+key+"]", depth+1)
			if err != nil {
				return nil, err
			}
			decoder, err := strictDecoder(item, name+"["+key+"]")
			if err != nil {
				return nil, typeError(paths.Join(path, key), n.object[key].origin, "invalid map decoder")
			}
			if err := decoder.Decode(child); err != nil {
				field := paths.Join(path, key)
				origin := n.object[key].origin
				var de *ms.DecodeError
				if errors.As(err, &de) {
					if mapped, ok := p.names[de.Name()]; ok {
						field = mapped
						if named, ok := p.origins[field]; ok {
							origin = named
						}
					}
				}
				reason := "invalid map value"
				var ce *ConfigError
				if errors.As(err, &ce) {
					reason = ce.Reason
				}
				return nil, typeError(field, origin, reason)
			}
			result[key] = item.Interface()
			target.SetMapIndex(k, item)
		}
		return result, nil
	case reflect.Slice, reflect.Array:
		items := n.list
		if marker, ok := n.value.(textvalue.Value); ok {
			parts := strings.Split(string(marker), ",")
			items = make([]*node, len(parts))
			for i, part := range parts {
				if part == "" {
					return nil, typeError(path, n.origin, "empty textual list element")
				}
				items[i] = &node{value: textvalue.Value(part), origin: n.origin}
			}
		}
		if items == nil {
			return inputValue(n), nil
		}
		if target.Kind() == reflect.Array && target.Len() != len(items) {
			return nil, typeError(path, n.origin, "array length mismatch")
		}
		if target.Kind() == reflect.Slice {
			target.Set(reflect.MakeSlice(target.Type(), len(items), len(items)))
		} else {
			target.Set(reflect.Zero(target.Type()))
		}
		result := make([]any, len(items))
		for i, item := range items {
			child, err := p.prepare(item, target.Index(i), fmt.Sprintf("%s[%d]", path, i), fmt.Sprintf("%s[%d]", name, i), depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = child
		}
		return result, nil
	default:
		return inputValue(n), nil
	}
}
func inputValue(n *node) any {
	if n.object != nil {
		out := map[string]any{}
		for key, child := range n.object {
			out[key] = inputValue(child)
		}
		return out
	}
	if n.list != nil {
		out := make([]any, len(n.list))
		for i, child := range n.list {
			out[i] = inputValue(child)
		}
		return out
	}
	return n.value
}
func hookError(reason string) error { return &ConfigError{Reason: reason} }
func durationHook(from, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeFor[time.Duration]() || from == to {
		return data, nil
	}
	var value string
	switch v := data.(type) {
	case string:
		value = v
	case textvalue.Value:
		value = string(v)
	default:
		return nil, hookError("duration requires string with unit")
	}
	if value == "0" {
		return nil, hookError("duration requires string with unit")
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return nil, hookError("invalid duration")
	}
	return duration, nil
}
func textHook(from, to reflect.Type, data any) (any, error) {
	if !textType(to) || from == to {
		return data, nil
	}
	var value string
	switch v := data.(type) {
	case string:
		value = v
	case textvalue.Value:
		value = string(v)
	default:
		return nil, hookError("text unmarshaler requires string")
	}
	fresh := reflect.New(to)
	if err := fresh.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(value)); err != nil {
		return nil, hookError("invalid text value")
	}
	return fresh.Elem().Interface(), nil
}
func numericHook(from, to reflect.Type, data any) (any, error) {
	if to == reflect.TypeFor[time.Duration]() {
		return data, nil
	}
	if number, ok := data.(json.Number); ok {
		if !numericKind(to.Kind()) {
			return nil, hookError("expected number target")
		}
		return checkedNumber(string(number), to)
	}
	if numericKind(to.Kind()) && from != to {
		if _, ok := data.(textvalue.Value); !ok {
			return nil, hookError("expected number")
		}
	}
	return data, nil
}
func textOriginHook(from, to reflect.Type, data any) (any, error) {
	marker, ok := data.(textvalue.Value)
	if !ok {
		return data, nil
	}
	value := string(marker)
	switch {
	case to.Kind() == reflect.String:
		return value, nil
	case to.Kind() == reflect.Bool:
		if value != "true" && value != "false" {
			return nil, hookError("expected boolean text")
		}
		return value == "true", nil
	case numericKind(to.Kind()):
		if !validNumber(value) {
			return nil, hookError("invalid decimal text")
		}
		return checkedNumber(value, to)
	case to.Kind() == reflect.Slice || to.Kind() == reflect.Array:
		parts := strings.Split(value, ",")
		out := make([]textvalue.Value, len(parts))
		for i, part := range parts {
			if part == "" {
				return nil, hookError("empty textual list element")
			}
			out[i] = textvalue.Value(part)
		}
		return out, nil
	default:
		return nil, hookError("unsupported textual conversion")
	}
}
func numericKind(kind reflect.Kind) bool { return kind >= reflect.Int && kind <= reflect.Float64 }
func checkedNumber(raw string, t reflect.Type) (any, error) {
	out := reflect.New(t).Elem()
	if t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64 {
		if strings.ContainsAny(raw, ".eE") {
			return nil, hookError("expected decimal integer")
		}
		value, err := strconv.ParseInt(raw, 10, t.Bits())
		if err != nil {
			return nil, hookError("integer overflow")
		}
		out.SetInt(value)
	} else if t.Kind() >= reflect.Uint && t.Kind() <= reflect.Uintptr {
		if strings.ContainsAny(raw, "-.eE") {
			return nil, hookError("expected unsigned decimal integer")
		}
		value, err := strconv.ParseUint(raw, 10, t.Bits())
		if err != nil {
			return nil, hookError("integer overflow")
		}
		out.SetUint(value)
	} else {
		value, err := strconv.ParseFloat(raw, t.Bits())
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
			return nil, hookError("float overflow")
		}
		if t.Kind() == reflect.Float32 {
			actual := new(big.Rat).SetFloat64(value).FloatString(149)
			sd, se, ss, ok := decimalParts(raw)
			ad, ae, as, _ := decimalParts(actual)
			if !ok || sd != ad || se != ae || ss != as {
				return nil, hookError("float32 loses precision")
			}
		}
		out.SetFloat(value)
	}
	return out.Interface(), nil
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
			if !field.IsExported() || keyName(field) == "-" {
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
			if f.IsExported() && keyName(f) != "-" {
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

func strictDecoder(target reflect.Value, name string) (*ms.Decoder, error) {
	return ms.NewDecoder(&ms.DecoderConfig{Result: target.Addr().Interface(), RootName: name, TagName: "mapstructure", WeaklyTypedInput: false, ZeroFields: false, ErrorUnused: true, MatchName: matchName, DisableUnmarshaler: true, DecodeHook: ms.ComposeDecodeHookFunc(durationHook, textHook, numericHook, textOriginHook, uintptrHook)})
}
func uintptrHook(from, to reflect.Value) (any, error) {
	if to.Kind() == reflect.Uintptr && from.Kind() == reflect.Uintptr {
		to.SetUint(from.Uint())
		return nil, nil
	}
	return from.Interface(), nil
}
