package thread

import (
	"context"
	"errors"
	"sync"

	"github.com/topfreegames/pitaya/v2/constants"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"github.com/topfreegames/pitaya/v2/lang"
	"github.com/topfreegames/pitaya/v2/util"
)

// ErrTaskRunnerBusy is the error that indicates the runner is busy.
var ErrTaskRunnerBusy = errors.New("task runner is busy")

// A TaskRunner is used to control the concurrency of goroutines.
type TaskRunner struct {
	limitChan chan lang.PlaceholderType
	waitGroup sync.WaitGroup
}

// NewTaskRunner returns a TaskRunner.
func NewTaskRunner(concurrency int) *TaskRunner {
	return &TaskRunner{
		limitChan: make(chan lang.PlaceholderType, concurrency),
	}
}

// Schedule schedules a task to run under concurrency control.
func (rp *TaskRunner) Schedule(task func()) {
	// Why we add waitGroup first, in case of race condition on starting a task and wait returns.
	// For example, limitChan is full, and the task is scheduled to run, but the waitGroup is not added,
	// then the wait returns, and the task is then scheduled to run, but caller thinks all tasks are done.
	// the same reason for ScheduleImmediately.
	rp.waitGroup.Add(1)
	rp.limitChan <- lang.Placeholder

	go func() {
		defer util.Recover(func() {
			<-rp.limitChan
			rp.waitGroup.Done()
		})

		task()
	}()
}

// Schedule schedules a task to run under concurrency control.
func (rp *TaskRunner) ScheduleWithCtx(ctx context.Context, task func(subCtx context.Context)) {
	// Why we add waitGroup first, in case of race condition on starting a task and wait returns.
	// For example, limitChan is full, and the task is scheduled to run, but the waitGroup is not added,
	// then the wait returns, and the task is then scheduled to run, but caller thinks all tasks are done.
	// the same reason for ScheduleImmediately.
	rp.waitGroup.Add(1)
	rp.limitChan <- lang.Placeholder

	md, _ := pcontext.FromPropagateContext(ctx)
	go func() {
		defer util.Recover(func() {
			<-rp.limitChan
			rp.waitGroup.Done()
		})

		subCtx := pcontext.NewPropagateContext(ctx, md)
		subCtx = context.WithValue(subCtx, constants.TaskIDKey, nil)

		task(subCtx)
	}()
}

// ScheduleImmediately schedules a task to run immediately under concurrency control.
// It returns ErrTaskRunnerBusy if the runner is busy.
func (rp *TaskRunner) ScheduleImmediately(task func()) error {
	// Why we add waitGroup first, check the comment in Schedule.
	rp.waitGroup.Add(1)
	select {
	case rp.limitChan <- lang.Placeholder:
	default:
		rp.waitGroup.Done()
		return ErrTaskRunnerBusy
	}

	go func() {
		defer util.Recover(func() {
			<-rp.limitChan
			rp.waitGroup.Done()
		})
		task()
	}()

	return nil
}

// Wait waits all running tasks to be done.
func (rp *TaskRunner) Wait() {
	rp.waitGroup.Wait()
}
