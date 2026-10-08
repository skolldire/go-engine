// Package http is the preset for a plain HTTP service.
//
// It registers no provider at all, so an application using it resolves the core
// dependency set only — no AWS SDK, no database drivers, no broker clients.
package http

import "github.com/skolldire/go-engine/pkg/engine"

// Options returns the options for an HTTP service with health probes.
//
//	eng, err := engine.New(ctx, http.Options()...)
func Options() []engine.Option {
	return []engine.Option{
		engine.WithRouter(),
		engine.WithHealth(),
	}
}
