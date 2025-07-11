package thread

import (
	"context"

	"github.com/topfreegames/pitaya/v2/constants"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"github.com/topfreegames/pitaya/v2/util"
)

// GoSafe executes a function in a goroutine and catches panics
func GoSafe(fn func()) {
	go RunSafe(fn)
}

// RunSafe executes a function and catches panics
func RunSafe(fn func()) {
	defer util.Recover()

	fn()
}

// GoSafeWithCtx executes a function in a goroutine and catches panics.
func GoSafeWithCtx(ctx context.Context, fn func(subCtx context.Context)) {
	md, _ := pcontext.FromPropagateContext(ctx)

	go func() {
		defer util.Recover()

		subCtx := pcontext.NewPropagateContext(ctx, md)
		subCtx = context.WithValue(subCtx, constants.TaskIDKey, nil)

		fn(subCtx)
	}()
}
