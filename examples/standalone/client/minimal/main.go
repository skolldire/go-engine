package main

import (
	"context"
	"errors"
	"fmt"

	c "github.com/skolldire/go-engine/client"
)

type key struct{}

func run() error {
	ctx := context.WithValue(context.Background(), key{}, "request")
	inv := c.Invocation{Component: "store", Operation: "Read"}
	domain := errors.New("domain")
	operation := c.Operation[string](func(received context.Context) (string, error) {
		if received != ctx || received.Value(key{}) != "request" {
			return "", errors.New("context changed")
		}
		return "record", domain
	})
	result, err := c.Do(ctx, inv, operation)
	if result != "record" || err != domain {
		return errors.New("Do changed result/error")
	}
	terminal := func(ctx context.Context, _ c.Invocation, op c.Operation[string]) (string, error) { return op(ctx) }
	chain, err := c.Chain(c.Handler[string](terminal))
	if err != nil {
		return err
	}
	result, err = chain(ctx, inv, operation)
	if result != "record" || err != domain {
		return errors.New("Chain changed result/error")
	}
	if _, err := c.Do(ctx, c.Invocation{}, operation); !errors.Is(err, c.ErrInvalidCall) {
		return errors.New("invalid invocation accepted")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("minimal: Do/Chain typed result, original context/error and validation OK")
}
