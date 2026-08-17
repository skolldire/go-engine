// Package viper loads and validates the legacy typed configuration.
//
// Deprecated: use [github.com/skolldire/go-engine/pkg/engine].Config instead.
//
// The Config struct in this package names the concrete Config type of every
// adapter, which is the single root cause of the library's coupling: a YAML
// reader that drags in forty modules. engine.Config keeps only the sections the
// core owns and carries the rest untouched via mapstructure's ",remain", so
// each provider decodes its own section.
//
// This package continues to work for consumers of the deprecated pkg/app.
package viper
