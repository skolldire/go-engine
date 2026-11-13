package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skolldire/go-engine/config/internal/paths"
	"github.com/skolldire/go-engine/config/internal/textvalue"
)

type resolved struct {
	value   string
	missing bool
}
type expander struct {
	ctx      context.Context
	resolver Resolver
	maxDepth int
	cache    map[string]resolved
	active   map[string]bool
}

func (e *expander) walk(n *node, path string, depth int, budget *measure) error {
	if depth > budget.maxDepth {
		return fmt.Errorf("%s: %w", path, ErrLimitExceeded)
	}
	if err := budget.add(1, path); err != nil {
		return err
	}
	if n.object != nil {
		for _, key := range sortedKeys(n.object) {
			child := n.object[key]
			if child.isNull() {
				delete(n.object, key)
				continue
			}
			field := paths.Join(path, key)
			if err := budget.add(int64(len(key)), field); err != nil {
				return err
			}
			if err := e.walk(child, field, depth+1, budget); err != nil {
				return err
			}
		}
	} else if n.list != nil {
		for i, child := range n.list {
			if err := e.walk(child, fmt.Sprintf("%s[%d]", path, i), depth+1, budget); err != nil {
				return err
			}
		}
	} else if value, ok := textContent(n.value); ok {
		output, textual, err := e.expand(value, path, n.origin, 0, budget.limit-budget.used)
		if err != nil {
			return err
		}
		_, marked := n.value.(textvalue.Value)
		n.value = output
		if marked || textual {
			n.value = textvalue.Value(output)
		}
		if err := budget.add(int64(len(output)), path); err != nil {
			return err
		}
	} else if n.value != nil {
		if value, ok := n.value.(interface{ String() string }); ok {
			if err := budget.add(int64(len(value.String())), path); err != nil {
				return err
			}
		}
	}
	return nil
}
func validReference(ref string) bool {
	scheme, name, hasScheme := strings.Cut(ref, ":")
	if !hasScheme {
		name = scheme
	} else {
		if scheme == "" || scheme[0] < 'a' || scheme[0] > 'z' {
			return false
		}
		for _, r := range scheme {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("+.-", r) {
				return false
			}
		}
	}
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("_./-", r) {
			return false
		}
	}
	return true
}
func referenceEnd(value string, start int) int {
	depth := 1
	for i := start; i < len(value); i++ {
		if strings.HasPrefix(value[i:], "$${") {
			i += 2
			continue
		}
		if strings.HasPrefix(value[i:], "${") {
			depth++
			i++
			continue
		}
		if value[i] == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
func (e *expander) expand(value, path, origin string, depth int, remaining int64) (string, bool, error) {
	if err := e.ctx.Err(); err != nil {
		return "", false, err
	}
	var out strings.Builder
	textual := false
	write := func(part string) error {
		if int64(len(part)) > remaining-int64(out.Len()) {
			return fmt.Errorf("%s: %w", path, ErrLimitExceeded)
		}
		out.WriteString(part)
		return nil
	}
	for i := 0; i < len(value); {
		if strings.HasPrefix(value[i:], "$${") {
			if err := write("${"); err != nil {
				return "", false, err
			}
			i += 3
			continue
		}
		if !strings.HasPrefix(value[i:], "${") {
			end := strings.IndexByte(value[i:], '$')
			if end < 0 {
				end = len(value) - i
			} else if end == 0 {
				end = 1
			}
			if err := write(value[i : i+end]); err != nil {
				return "", false, err
			}
			i += end
			continue
		}
		end := referenceEnd(value, i+2)
		if end < 0 {
			return "", false, &ConfigError{path, "unclosed reference from " + origin}
		}
		ref, fallback, hasFallback := strings.Cut(value[i+2:end], ":-")
		if !validReference(ref) {
			return "", false, &ConfigError{path, "invalid reference from " + origin}
		}
		if depth >= e.maxDepth {
			return "", false, fmt.Errorf("%s: %w", path, ErrLimitExceeded)
		}
		if e.active[ref] {
			return "", false, fmt.Errorf("%s: %w", path, ErrReferenceCycle)
		}
		if e.resolver == nil {
			return "", false, fmt.Errorf("%s: %w", path, ErrUnresolvedReference)
		}
		result, cached := e.cache[ref]
		if !cached {
			raw, err := e.resolver.Resolve(e.ctx, ref)
			if err != nil && !errors.Is(err, ErrReferenceNotFound) {
				return "", false, fmt.Errorf("resolve %s: %w", path, err)
			}
			result = resolved{value: raw, missing: errors.Is(err, ErrReferenceNotFound)}
			e.cache[ref] = result
		}
		raw := result.value
		if result.missing {
			if !hasFallback {
				return "", false, fmt.Errorf("%s: %w", path, ErrUnresolvedReference)
			}
			raw = fallback
		}
		e.active[ref] = true
		expanded, _, err := e.expand(raw, path, origin, depth+1, remaining-int64(out.Len()))
		delete(e.active, ref)
		if err != nil {
			return "", false, err
		}
		if err := write(expanded); err != nil {
			return "", false, err
		}
		textual = true
		i = end + 1
	}
	return out.String(), textual, nil
}

func textContent(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case textvalue.Value:
		return string(v), true
	default:
		return "", false
	}
}
