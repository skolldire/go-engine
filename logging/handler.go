package logging

import (
	"context"
	"log/slog"
	"slices"
	"strings"
)

type policy struct {
	keys     []string
	maxBytes int
	redactor Redactor
}

func (p *policy) sensitive(key string) bool {
	for _, candidate := range p.keys {
		if strings.EqualFold(candidate, key) {
			return true
		}
	}
	return false
}

type handler struct {
	output  slog.Handler
	policy  *policy
	groups  []string
	blocked bool
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.output.Enabled(ctx, level)
}
func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	transformed := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	if !h.blocked {
		r.Attrs(func(a slog.Attr) bool {
			n := normalizer{policy: h.policy}
			transformed.AddAttrs(n.attr(a, h.groups, 1))
			return true
		})
	}
	return h.output.Handle(ctx, transformed)
}
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h.blocked || len(attrs) == 0 {
		return h
	}
	transformed := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		n := normalizer{policy: h.policy}
		transformed = append(transformed, n.attr(a, h.groups, 1))
	}
	return &handler{output: h.output.WithAttrs(transformed), policy: h.policy, groups: h.groups}
}
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" || h.blocked {
		return h
	}
	if h.policy.sensitive(name) {
		return &handler{output: h.output.WithAttrs([]slog.Attr{slog.String(name, redacted)}), policy: h.policy, groups: h.groups, blocked: true}
	}
	return &handler{output: h.output.WithGroup(name), policy: h.policy, groups: append(slices.Clone(h.groups), name)}
}
