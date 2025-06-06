package thread

import (
	"runtime/debug"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
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

// SafeRunWithTimeout executes a function and catches panics and timeouts
func SafeRunWithTimeout(fn func(), timeout time.Duration) {
	defer util.Recover()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		if elapsed > timeout {
			logger.Log.Warnf("[DELAY] task timeout: took %v > %v\n%s", elapsed, timeout, debug.Stack())
		}
	}()

	fn()
}
