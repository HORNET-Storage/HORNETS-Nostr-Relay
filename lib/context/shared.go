package context

import "context"

var Context context.Context

func GetContext() context.Context {
	return Context
}

func SetContext(ctx context.Context) {
	Context = ctx
}
