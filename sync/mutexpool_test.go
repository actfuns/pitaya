package sync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMutexPool_Basic(t *testing.T) {
	pool := NewMutexPool[string]()

	var counter int

	// 同一个 key 多次 WithLock
	for i := 0; i < 5; i++ {
		pool.WithLock("key1", func() {
			// mu.Lock()
			counter++
			// mu.Unlock()
		})
	}

	if counter != 5 {
		t.Errorf("expected counter=5, got %d", counter)
	}
}

func TestMutexPool_ConcurrentSameKey(t *testing.T) {
	pool := NewMutexPool[string]()
	var counter int

	const N = 1000000
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.WithLock("shared", func() {
				counter++
			})
		}()
	}

	wg.Wait()
	if counter != N {
		t.Errorf("expected counter=%d, got %d", N, counter)
	}
}

func TestMutexPool_ConcurrentDifferentKeys(t *testing.T) {
	pool := NewMutexPool[int]()
	var counters [10]int
	var mu sync.Mutex

	const N = 100
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		for k := 0; k < 10; k++ {
			wg.Add(1)
			go func(key int) {
				defer wg.Done()
				pool.WithLock(key, func() {
					mu.Lock()
					counters[key]++
					mu.Unlock()
				})
			}(k)
		}
	}

	wg.Wait()
	for i, c := range counters {
		if c != N {
			t.Errorf("counter[%d] expected %d, got %d", i, N, c)
		}
	}
}

func TestMutexPool_RecycleAndReuse(t *testing.T) {
	pool := NewMutexPool[string]()

	// 第一轮：创建 key1
	var ptr1 *RefMutex
	pool.WithLock("key1", func() {
		ptr1 = pool.getOrAdd("key1")
	})

	// 此时 refCount 应为 0，但对象可能还在 map 中
	time.Sleep(10 * time.Millisecond) // 模拟释放窗口

	// 第二轮：再次使用 key1
	var ptr2 *RefMutex
	pool.WithLock("key1", func() {
		ptr2 = pool.getOrAdd("key1")
	})

	// 检查是否是同一个指针（你的核心诉求）
	if ptr1 != ptr2 {
		t.Errorf("expected same mutex pointer after reuse, but got different ones")
	}
}

func TestMutexPool_RefCountCorrectness(t *testing.T) {
	pool := NewMutexPool[string]()
	rm := pool.getOrAdd("test")
	if atomic.LoadInt32(&rm.refCount) != 1 {
		t.Errorf("refCount should be 1 after getOrAdd, got %d", rm.refCount)
	}

	// 模拟释放
	pool.release("test", rm)
	if atomic.LoadInt32(&rm.refCount) != 0 {
		t.Errorf("refCount should be 0 after release, got %d", rm.refCount)
	}

	// 再次获取
	rm2 := pool.getOrAdd("test")
	if atomic.LoadInt32(&rm2.refCount) != 1 {
		t.Errorf("refCount should be 1 after reuse, got %d", rm2.refCount)
	}

	// 检查是否是同一个对象
	if rm == rm2 {
		t.Errorf("expected same mutex instance, got different")
	}
}

// ----------------------------
// 压力测试 + race 检测
// 运行：go test -v -race mutexpool_test.go
// ----------------------------

func TestMutexPool_Stress(t *testing.T) {
	pool := NewMutexPool[string]()
	var counter int
	var mu sync.Mutex

	const (
		Keys       = 10
		Goroutines = 100
		OpsPerG    = 1000
	)

	var wg sync.WaitGroup

	for i := 0; i < Goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < OpsPerG; j++ {
				key := string('a' + byte(j%Keys))
				pool.WithLock(key, func() {
					mu.Lock()
					counter++
					mu.Unlock()
				})
			}
		}()
	}

	wg.Wait()
	expected := Goroutines * OpsPerG
	if counter != expected {
		t.Errorf("stress test: expected %d, got %d", expected, counter)
	}
}
