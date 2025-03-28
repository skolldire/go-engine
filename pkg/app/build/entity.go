package build

import "context"

type App interface {
	Run() error
}

type Builder interface {
	Build() App
	SetContext(context.Context) Builder
}
