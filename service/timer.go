package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/timer"
)

type timerEntity struct {
	taskid  string
	counter int32
	fn      func(context.Context)
	delay   time.Duration
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

func (ts *TimerService) SetInterval(taskid string, delay time.Duration, counter int32, fn func(context.Context)) (uint64, error) {
	timerId := atomic.AddUint64(&ts.id, 1)
	if err := ts.timingWheel.SetTimer(timerId, &timerEntity{
		taskid:  taskid,
		fn:      fn,
		counter: counter,
		delay:   delay,
	}, delay); err != nil {
		return 0, err
	}
	return timerId, nil
}

func (ts *TimerService) ClearInterval(timerId uint64) error {
	return ts.timingWheel.RemoveTimer(timerId)
}

func (ts *TimerService) execute(key, value any) {
	task := value.(*timerEntity)
	ts.taskService.Submit(context.Background(), task.taskid, task.fn)
	if task.counter != -1 {
		newCounter := atomic.AddInt32(&task.counter, -1)
		if newCounter == 0 {
			return
		}
	}
	ts.timingWheel.SetTimer(key, task, task.delay)
}

func (ts *TimerService) Shutdown() {
	ts.timingWheel.Drain(func(key, value any) {})
	logger.Log.Infof("timerService stopped!")
}
