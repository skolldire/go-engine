package config

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"

	"github.com/skolldire/go-engine/config/internal/paths"
	"github.com/skolldire/go-engine/config/internal/textvalue"
)

type node struct {
	value  any
	object map[string]*node
	list   []*node
	origin string
}
type measure struct {
	limit, used int64
	maxDepth    int
	ctx         context.Context
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
		return v.IsNil()
	}
	return false
}
func (m *measure) add(n int64, path string) error {
	if err := m.ctx.Err(); err != nil {
		return err
	}
	if n > m.limit-m.used {
		return fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	m.used += n
	return nil
}
func (m *measure) copy(value any, path, origin string, depth int) (*node, error) {
	if depth > m.maxDepth {
		return nil, fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	if err := m.add(1, path); err != nil {
		return nil, err
	}
	n := &node{origin: origin}
	switch v := value.(type) {
	case nil, bool:
		n.value = v
	case string:
		n.value = v
		if err := m.add(int64(len(v)), path); err != nil {
			return nil, err
		}
	case textvalue.Value:
		n.value = v
		if err := m.add(int64(len(v)), path); err != nil {
			return nil, err
		}
	case json.Number:
		if !validNumber(string(v)) {
			return nil, fmt.Errorf("%s: %w", path, ErrInvalidDocument)
		}
		n.value = v
		if err := m.add(int64(len(v)), path); err != nil {
			return nil, err
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("%s: %w", path, ErrInvalidDocument)
		}
		number := strconv.FormatFloat(v, 'g', -1, 64)
		n.value = json.Number(number)
		if err := m.add(int64(len(number)), path); err != nil {
			return nil, err
		}
	case map[string]any:
		if int64(len(v)) > m.limit-m.used {
			return nil, fmt.Errorf("%s: %w", path, ErrLimitExceeded)
		}
		n.object = make(map[string]*node)
		for _, key := range sortedKeys(v) {
			field := paths.Join(path, key)
			if err := m.add(int64(len(key)), field); err != nil {
				return nil, err
			}
			child, err := m.copy(v[key], field, origin, depth+1)
			if err != nil {
				return nil, err
			}
			n.object[key] = child
		}
	case []any:
		if int64(len(v)) > m.limit-m.used {
			return nil, fmt.Errorf("%s: %w", path, ErrLimitExceeded)
		}
		n.list = make([]*node, 0)
		for i, item := range v {
			child, err := m.copy(item, fmt.Sprintf("%s[%d]", path, i), origin, depth+1)
			if err != nil {
				return nil, err
			}
			n.list = append(n.list, child)
		}
	default:
		rv := reflect.ValueOf(value)
		var number string
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			number = strconv.FormatInt(rv.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			number = strconv.FormatUint(rv.Uint(), 10)
		default:
			return nil, fmt.Errorf("%s: %w", path, ErrInvalidDocument)
		}
		n.value = json.Number(number)
		if err := m.add(int64(len(number)), path); err != nil {
			return nil, err
		}
	}
	return n, nil
}
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func validNumber(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[i] == '-' {
		i++
		if i == len(s) {
			return false
		}
	}
	if s[i] == '0' {
		i++
	} else {
		if s[i] < '1' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && s[i] == '.' {
		i++
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}
func (n *node) isNull() bool { return n.object == nil && n.list == nil && n.value == nil }
func merge(target, incoming *node, path string) error {
	for _, key := range sortedKeys(incoming.object) {
		next := incoming.object[key]
		previous, exists := target.object[key]
		field := paths.Join(path, key)
		if next.isNull() {
			delete(target.object, key)
			continue
		}
		if !exists {
			if next.object != nil {
				empty := &node{object: map[string]*node{}, origin: next.origin}
				if err := merge(empty, next, field); err != nil {
					return err
				}
				target.object[key] = empty
			} else {
				target.object[key] = next
			}
			continue
		}
		if (previous.object != nil) != (next.object != nil) || (previous.list != nil) != (next.list != nil) {
			return &ConfigError{field, "incompatible shapes from " + previous.origin + " and " + next.origin}
		}
		if next.object != nil {
			if err := merge(previous, next, field); err != nil {
				return err
			}
		} else {
			target.object[key] = next
		}
	}
	return nil
}
