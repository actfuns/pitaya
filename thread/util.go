package thread

import (
	"fmt"
	"reflect"
	"runtime"
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

// RunSafeWithTimeout executes a function and catches panics and timeouts
func RunSafeWithTimeout(fn func(), timeout time.Duration) {
	if fn == nil {
		return
	}
	defer util.Recover()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		if elapsed > timeout {
			fnName := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
			_, file, line, ok := runtime.Caller(2)
			callerInfo := "unknown"
			if ok {
				callerInfo = fmt.Sprintf("%s:%d", file, line)
			}
			logger.Log.Warnf("[DELAY] safe run task timeout. elapsed: %v, timeout: %v, fn: %s, caller: %s", elapsed, timeout, fnName, callerInfo)
		}
	}()

	fn()
}
