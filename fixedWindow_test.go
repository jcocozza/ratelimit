package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestFixedWindow_checkAndDoReset(t *testing.T) {
	tests := []struct {
		name        string
		sleepTime   time.Duration
		wantSuccess bool
	}{
		{"test 1", 2 * time.Second, true},
		{"test 2", 500 * time.Millisecond, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := NewFixedWindow(1*time.Second, 2)
			// make requests
			for i := 0; i < 2; i++ {
				if err := fw.Request(); err != nil {
					t.Errorf("[%s] Request() = %v, total requests: %d", fw, err, fw.count)
				}
			}
			// limit reached
			if err := fw.Request(); err == nil {
				t.Errorf("[%s] expected limit error, got none, total requests: %d", fw, fw.count)
			}
			// restore limit
			time.Sleep(tt.sleepTime)
			// should be able to make request after restore
			if err := fw.Request(); (err != nil) != !tt.wantSuccess {
				t.Errorf("[%s] request after reset = %v, wantSuccess = %v, total requests: %d", fw, err, tt.wantSuccess, fw.count)
			}

		})
	}
}

func TestFixedWindow_Request(t *testing.T) {
	tests := []struct {
		name              string
		fw                *FixedWindow
		numRequestsToMake int
		wantError         bool
	}{
		{"test 1", NewFixedWindow(2*time.Second, 3), 3, false},
		{"test 2", NewFixedWindow(2*time.Second, 4), 5, true},
		{"test 3", NewFixedWindow(time.Minute, 100), 100, false},
		{"test 4", NewFixedWindow(time.Minute, 100), 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for i := 0; i < tt.numRequestsToMake; i++ {
				err = tt.fw.Request()
				if err != nil {
					break
				}
			}
			if (err != nil) != tt.wantError {
				t.Errorf("[%s] Request() = %v, wantError = %v, total requests: %d", tt.fw, err, tt.wantError, tt.fw.count)
			}
		})
	}
}

func TestFixedWindow_WaitRequest(t *testing.T) {
	tests := []struct {
		name              string
		numRequestsToMake int
		cancelContext     bool
		wantError         bool
	}{
		{"test 1", 3, false, false},
		{"test 2", 5, false, false},
		{"test 3", 1, true, true},
		{"test 4", 100, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := NewFixedWindow(time.Second, 3)
			if tt.cancelContext {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				err := fw.WaitRequest(ctx)
				if (err != nil) != tt.wantError {
					t.Errorf("[%s] Request() = %v, wantError = %v, total requests: %d", fw, err, tt.wantError, fw.count)
				}
			} else {
				for i := 0; i < tt.numRequestsToMake; i++ {
					err := fw.WaitRequest(context.Background())
					if (err != nil) != tt.wantError && i+1 == tt.numRequestsToMake {
						t.Errorf("[%s] Request() = %v, wantError = %v, total requests: %d", fw, err, tt.wantError, fw.count)
					}
				}
			}
		})
	}
}

func TestFixedWindow_ConcurrentRequests(t *testing.T) {
	tests := []struct {
		name            string
		concurrentCount int
		wantMaxSuccess  int
	}{
		{"More than max requests", 10, 5},
		{"less than max requests", 2, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := NewFixedWindow(1*time.Second, 5)

			var wg sync.WaitGroup
			successCount := 0
			for i := 0; i < tt.concurrentCount; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := fw.Request(); err == nil {
						successCount++
					}
				}()
			}
			wg.Wait()

			// Ensure we do not exceed the maximum allowed requests
			if successCount > fw.MaxRequests {
				t.Errorf("Exceeded max requests: got %d, expected max %d", successCount, fw.MaxRequests)
			}
		})
	}
}

/*
func TestFixedWindow_CommonUseCase(t *testing.T) {
	fwShort := NewFixedWindow(1*time.Second, 10)
	fwLong := NewFixedWindow(30*time.Second, 30)

	ctx := context.Background()
	check := func() error {
		err := fwLong.WaitRequest(ctx)
		if err != nil {
			return err
		}
		err = fwShort.WaitRequest(ctx)
		if err != nil {
			return err
		}
		return nil
	}

	const numWork = 100
	work := make(chan string, numWork)
	completedWork := make(chan string, numWork)

	producer := func() {
		for i := 0; i < numWork; i++ {
			work <- fmt.Sprint(i)
		}
		close(work)
		fmt.Println("producer done")
		fmt.Println("work has elements: ", len(work))
	}
	worker := func(wg *sync.WaitGroup) {
		defer wg.Done()
		for elm := range work {
			for {
				// worker can only do work when it has permission from both windows to do so
				err := check()
				if err == nil {
					// doing work
					time.Sleep(time.Second)
					fmt.Printf("pushing %s...\n", elm)
					completedWork <- elm
					break
				} else { // wait and try again
					fmt.Println("err: ", err.Error())
					time.Sleep(time.Second)
				}
			}
		}
	}

	t.Run("common use case", func(t *testing.T) {
		fmt.Println("running common use case...")
		go producer()
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go worker(&wg)
		}
		wg.Wait()
	})
}
*/
