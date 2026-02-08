package cli

// applyMiddleware wraps fn with the given middleware in order.
// Middleware is applied so that the first middleware in the slice is the
// outermost wrapper (executes first).
func applyMiddleware(fn RunFunc, mw []func(next RunFunc) RunFunc) RunFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		fn = mw[i](fn)
	}
	return fn
}
