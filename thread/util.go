package thread

import (
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
