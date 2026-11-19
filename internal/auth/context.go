package auth

import "context"

type ctxKey int

const accountCtxKey ctxKey = 1

// ContextWithAccount attaches the authenticated account to ctx.
func ContextWithAccount(ctx context.Context, a Account) context.Context {
	return context.WithValue(ctx, accountCtxKey, a)
}

// AccountFromContext returns the account loaded by session middleware, if any.
func AccountFromContext(ctx context.Context) (Account, bool) {
	a, ok := ctx.Value(accountCtxKey).(Account)
	return a, ok
}
