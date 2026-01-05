package build

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockBuilder struct {
	ctx context.Context
}

func (m *mockBuilder) Build() App {
	return &mockApp{}
}

func (m *mockBuilder) SetContext(ctx context.Context) Builder {
	m.ctx = ctx
	return m
}

type mockApp struct{}

func (m *mockApp) Run() error {
	return nil
}

func TestApply(t *testing.T) {
	builder := &mockBuilder{}
	app := Apply(builder)
	assert.NotNil(t, app)
}

func TestApplyWithContext(t *testing.T) {
	builder := &mockBuilder{}
	ctx := context.Background()
	app := ApplyWithContext(ctx, builder)
	assert.NotNil(t, app)
	assert.Equal(t, ctx, builder.ctx)
}

func TestMockApp_Run(t *testing.T) {
	app := &mockApp{}
	err := app.Run()
	assert.NoError(t, err)
}

