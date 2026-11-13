package main

import (
	"github.com/skolldire/go-engine/circuitbreaker"
	"github.com/skolldire/go-engine/concurrency"
	"github.com/skolldire/go-engine/ratelimit"
	"github.com/skolldire/go-engine/resilience"
)

var (
	_ resilience.RateLimiter = (*ratelimit.Limiter)(nil)
	_ resilience.Limiter     = (*concurrency.Limiter)(nil)
	_ resilience.Breaker     = (*circuitbreaker.Breaker)(nil)
)
