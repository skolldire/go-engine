package otel

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func apiMetricAttrs(i int) metric.AddOption { return metric.WithAttributes(attribute.Int("id", i)) }
