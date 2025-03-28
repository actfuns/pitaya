package thread

import (
	"runtime/debug"
	"strconv"

	"github.com/topfreegames/pitaya/v2/logger"
)

// GoSafe executes a function in a goroutine and catches panics
func GoSafe(fn func()) {
	go RunSafe(fn)
}

// RunSafe executes a function and catches panics
func RunSafe(fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			stackTrace := debug.Stack()
			stackTraceAsRawStringLiteral := strconv.Quote(string(stackTrace))
			logger.Log.Errorf("panic - RunSafe: panicData=%v stackTrace=%s", rec, stackTraceAsRawStringLiteral)
		}
	}()
	fn()
}
