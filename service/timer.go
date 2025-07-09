package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/timer"
)

type timerEntity struct {
	taskid string
	fn     func(context.Context)
}

type TimerService struct {
	id          uint64
	taskService *TaskService
	timingWheel *timer.TimingWheel
}

func NewTimerService(interval time.Duration, numSlots int, taskService *TaskService) (*TimerService, error) {
	ts := TimerService{
		id:          0,
		taskService: taskService,
	}
	timingWheel, err := timer.NewTimingWheelWithTicker(interval, numSlots, ts.execute, timer.NewTicker(interval))
	if err != nil {
		return nil, err
	}
	ts.timingWheel = timingWheel
	return &ts, nil
}

func (ts *TimerService) SetInterval(taskid string, delay time.Duration, counter int, fn func(context.Context)) (uint64, error) {
	if counter == 0 {
		return 0, timer.ErrArgument
	}
	timerId := atomic.AddUint64(&ts.id, 1)
	if err := ts.timingWheel.SetTimer(timerId, &timerEntity{
		taskid: taskid,
		fn:     fn,
	}, delay, int(counter)); err != nil {
		return 0, err
	}
	return timerId, nil
}

func (ts *TimerService) ClearInterval(timerId uint64) error {
	return ts.timingWheel.RemoveTimer(timerId)
}

func (ts *TimerService) execute(key, value any) {
	task := value.(*timerEntity)
	if err := ts.taskService.Submit(context.Background(), task.taskid, task.fn); err != nil {
		logger.Log.Errorf("timerService task %s execute error: %v", task.taskid, err)
	}
}

func (ts *TimerService) Shutdown() {
	ts.timingWheel.Drain(func(key, value any) {})
	logger.Log.Infof("timerService stopped!")
}
