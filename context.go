package relay

import "context"

type ctxKeySkipTotalCount struct{}

func WithSkipTotalCount(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipTotalCount{}, true)
}

func ShouldSkipTotalCount(ctx context.Context) bool {
	b, _ := ctx.Value(ctxKeySkipTotalCount{}).(bool)
	return b
}

type ctxKeySkipEdges struct{}

func WithSkipEdges(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipEdges{}, true)
}

func ShouldSkipEdges(ctx context.Context) bool {
	b, _ := ctx.Value(ctxKeySkipEdges{}).(bool)
	return b
}

type ctxKeySkipNodes struct{}

func WithSkipNodes(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipNodes{}, true)
}

func ShouldSkipNodes(ctx context.Context) bool {
	b, _ := ctx.Value(ctxKeySkipNodes{}).(bool)
	return b
}
