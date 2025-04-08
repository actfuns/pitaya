package service

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSetTimer(t *testing.T) {
	taskService, _ := NewTaskService(1, 1, 1)
	timerService, _ := NewTimerService(time.Millisecond*10, 1000, taskService)
	last := time.Now().UnixMilli()
	timerService.SetInterval("test", time.Millisecond*400, -1, func(ctx context.Context) {
		now := time.Now().UnixMilli()
		fmt.Printf("%d\n", now-last)
		last = now
	})
	// go func() {
	// 	t := time.NewTicker(time.Millisecond * 400)
	// 	last := time.Now().UnixMilli()
	// 	for range t.C {
	// 		now := time.Now().UnixMilli()
	// 		fmt.Printf("2 %d\n", now-last)
	// 		last = now
	// 	}
	// }()
	time.Sleep(time.Second * 20)
}
