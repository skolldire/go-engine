package logging

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	redacted       = "[REDACTED]"
	depthLimit     = 16
	itemLimit      = 256
	nodeLimit      = 4096
	truncatedDepth = "[TRUNCATED_DEPTH]"
	truncatedItems = "[TRUNCATED_ITEMS]"
	truncatedSize  = "[TRUNCATED_SIZE]"
	cycleMarker    = "[CYCLE]"
	unsupported    = "[UNSUPPORTED]"
	unserializable = "[UNSERIALIZABLE]"
	truncation     = "…(truncated)"
)

type identity struct {
	kind    reflect.Kind
	typ     reflect.Type
	address uintptr
	length  int
}
type normalizer struct {
	policy *policy
	nodes  int
	path   map[identity]bool
}

func (n *normalizer) limit(depth int) string {
	if depth > depthLimit {
		return truncatedDepth
	}
	if n.nodes >= nodeLimit {
		return truncatedSize
	}
	n.nodes++
	return ""
}
func empty(a slog.Attr) bool {
	return a.Key == "" && a.Value.Kind() == slog.KindAny && a.Value.Any() == nil
}
func resolve(v slog.Value) slog.Value {
	if v.Kind() == slog.KindLogValuer {
		value := reflect.ValueOf(v.Any())
		if value.Kind() == reflect.Pointer && value.IsNil() {
			return slog.AnyValue(nil)
		}
	}
	return v.Resolve()
}
func (n *normalizer) attr(a slog.Attr, groups []string, depth int) slog.Attr {
	if empty(a) {
		return slog.Attr{}
	}
	if marker := n.limit(depth); marker != "" {
		return slog.String(a.Key, marker)
	}
	a.Value = resolve(a.Value)
	if a.Value.Kind() != slog.KindGroup && n.policy.redactor != nil {
		a = n.policy.redactor(slices.Clone(groups), a)
		if empty(a) {
			return slog.Attr{}
		}
		a.Value = resolve(a.Value)
	}
	if n.policy.sensitive(a.Key) {
		return slog.String(a.Key, redacted)
	}
	if a.Value.Kind() == slog.KindGroup {
		path := groups
		if a.Key != "" {
			path = append(slices.Clone(groups), a.Key)
		}
		attrs := a.Value.Group()
		result := make([]slog.Attr, 0, min(len(attrs), nodeLimit-n.nodes+1))
		for _, child := range attrs {
			if n.nodes >= nodeLimit {
				result = append(result, slog.String(truncatedSize, truncatedSize))
				break
			}
			result = append(result, n.attr(child, path, depth+1))
		}
		a.Value = slog.GroupValue(result...)
		return a
	}
	a.Value = slog.AnyValue(n.slogValue(a.Value, depth))
	return a
}
func (n *normalizer) slogValue(v slog.Value, depth int) any {
	switch v.Kind() {
	case slog.KindString:
		return n.text(v.String())
	case slog.KindAny:
		return n.value(reflect.ValueOf(v.Any()), depth, false)
	default:
		return v.Any()
	}
}
func (n *normalizer) text(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= n.policy.maxBytes {
		return s
	}
	marker := prefix(truncation, n.policy.maxBytes)
	return prefix(s, n.policy.maxBytes-len(marker)) + marker
}
func prefix(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}
func (n *normalizer) value(v reflect.Value, depth int, count bool) any {
	if count {
		if marker := n.limit(depth); marker != "" {
			return marker
		}
	}
	if !v.IsValid() {
		return nil
	}
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice {
		id := identity{kind: v.Kind(), typ: v.Type(), address: uintptr(v.UnsafePointer())}
		if v.Kind() == reflect.Slice {
			id.length = v.Len()
		}
		if n.path[id] {
			return cycleMarker
		}
		if n.path == nil {
			n.path = make(map[identity]bool)
		}
		n.path[id] = true
		defer delete(n.path, id)
	}
	if v.CanInterface() {
		x := v.Interface()
		switch x := x.(type) {
		case time.Time:
			return x
		case *time.Time:
			return *x
		case time.Duration:
			return x
		case *time.Duration:
			return *x
		case json.Number:
			return x
		case *json.Number:
			return *x
		case slog.Value:
			return n.resolved(x, depth)
		case slog.LogValuer:
			return n.resolved(slog.AnyValue(x).Resolve(), depth)
		case json.Marshaler:
			return n.marshal(x, depth)
		case error:
			return n.text(x.Error())
		case fmt.Stringer:
			return n.text(x.String())
		case encoding.TextMarshaler:
			b, err := x.MarshalText()
			if err != nil {
				return unserializable
			}
			return n.text(string(b))
		case []byte:
			return n.text(base64.StdEncoding.EncodeToString(x))
		}
	}
	switch v.Kind() {
	case reflect.Pointer:
		return n.value(v.Elem(), depth, true)
	case reflect.Bool:
		return v.Bool()
	case reflect.String:
		return n.text(v.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Map:
		return n.object(v, depth)
	case reflect.Struct:
		return n.structure(v, depth)
	case reflect.Slice, reflect.Array:
		output := make([]any, 0, min(v.Len(), itemLimit)+1)
		for i := 0; i < min(v.Len(), itemLimit); i++ {
			if n.nodes >= nodeLimit {
				output = append(output, truncatedSize)
				return output
			}
			output = append(output, n.value(v.Index(i), depth+1, true))
		}
		if v.Len() > itemLimit {
			output = append(output, truncatedItems)
		}
		return output
	default:
		return unsupported
	}
}
func (n *normalizer) resolved(v slog.Value, depth int) any {
	v = v.Resolve()
	if v.Kind() == slog.KindGroup {
		result := make(map[string]any)
		for _, a := range v.Group() {
			if n.nodes >= nodeLimit {
				result[truncatedSize] = truncatedSize
				break
			}
			if n.policy.sensitive(a.Key) {
				n.nodes++
				result[a.Key] = redacted
				continue
			}
			result[a.Key] = n.value(reflect.ValueOf(a.Value), depth+1, true)
		}
		return result
	}
	return n.slogValue(v, depth)
}
func (n *normalizer) marshal(m json.Marshaler, depth int) any {
	raw, err := m.MarshalJSON()
	if err != nil {
		return unserializable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var x any
	if err = decoder.Decode(&x); err != nil {
		return unserializable
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return unserializable
	}
	return n.value(reflect.ValueOf(x), depth, false)
}

type entry struct {
	key   string
	value reflect.Value
}

func (n *normalizer) object(v reflect.Value, depth int) any {
	kind := v.Type().Key().Kind()
	switch kind {
	case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
	default:
		return unsupported
	}
	entries := make([]entry, 0, min(v.Len(), itemLimit))
	iter := v.MapRange()
	for iter.Next() {
		key := iter.Key()
		var name string
		switch kind {
		case reflect.String:
			name = key.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			name = strconv.FormatInt(key.Int(), 10)
		default:
			name = strconv.FormatUint(key.Uint(), 10)
		}
		i := sort.Search(len(entries), func(i int) bool { return entries[i].key >= name })
		if i >= itemLimit {
			continue
		}
		if len(entries) < itemLimit {
			entries = append(entries, entry{})
		}
		copy(entries[i+1:], entries[i:len(entries)-1])
		entries[i] = entry{key: name, value: iter.Value()}
	}
	output := make(map[string]any, len(entries)+1)
	for _, e := range entries {
		if n.nodes >= nodeLimit {
			output[truncatedSize] = truncatedSize
			return output
		}
		if n.policy.sensitive(e.key) {
			n.nodes++
			output[e.key] = redacted
		} else {
			output[e.key] = n.value(e.value, depth+1, true)
		}
	}
	if v.Len() > itemLimit {
		output[truncatedItems] = truncatedItems
	}
	return output
}
func (n *normalizer) structure(v reflect.Value, depth int) any {
	output := make(map[string]any)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		if n.nodes >= nodeLimit {
			output[truncatedSize] = truncatedSize
			break
		}
		if n.policy.sensitive(name) || n.policy.sensitive(field.Name) {
			n.nodes++
			output[name] = redacted
		} else {
			output[name] = n.value(v.Field(i), depth+1, true)
		}
	}
	return output
}
