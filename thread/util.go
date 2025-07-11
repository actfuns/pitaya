package thread

import (
	"context"
	"maps"

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
	m := pcontext.ToMap(ctx)
	m2 := make(map[string]any, len(m))
	maps.Copy(m2, m)

	go func() {
		defer util.Recover()

		subCtx := context.WithValue(ctx, constants.PropagateCtxKey, m2)
		subCtx = context.WithValue(subCtx, constants.TaskIDKey, nil)
		fn(subCtx)
	}()
}
