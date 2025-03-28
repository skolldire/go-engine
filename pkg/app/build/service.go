package build

import "context"

func Apply(builder Builder) App {
	return builder.Build()
}

func ApplyWithContext(ctx context.Context, builder Builder) App {
	return builder.SetContext(ctx).Build()
}
