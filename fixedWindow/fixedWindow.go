package fixedwindow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type FixedWindow struct {
	MaxRequests int
	Interval    time.Duration
	count       int
	resetAt     time.Time
	mu          sync.Mutex
}

func NewFixedWindow(interval time.Duration, maxRequests int) *FixedWindow {
	return &FixedWindow{
		MaxRequests: maxRequests,
		Interval:    interval,
		resetAt:     time.Now().Add(interval),
	}
}

func (fw *FixedWindow) String() string {
	return fmt.Sprintf("Max Requests: %d, Interval: %s", fw.MaxRequests, fw.Interval.String())
}

func (fw *FixedWindow) needReset() bool {
	return time.Now().After(fw.resetAt)
}

func (fw *FixedWindow) resetCount() {
	fw.count = 0
	for !fw.resetAt.After(time.Now()) {
		fw.resetAt = fw.resetAt.Add(fw.Interval)
	}
}

func (fw *FixedWindow) checkAndDoReset() {
	if fw.needReset() {
		fw.resetCount()
	}
}

func (fw *FixedWindow) limitReached() bool {
	return fw.count >= fw.MaxRequests
}

func (fw *FixedWindow) timeUntilMakeRequest() time.Duration {
	fw.checkAndDoReset()
	if !fw.limitReached() {
		return 0
	}
	f := time.Until(fw.resetAt)
	return f
}

// return the number of requests left in the window
func (fw *FixedWindow) RequestsRemaining() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.MaxRequests - fw.count
}

// Return the duration until the window will reset
func (fw *FixedWindow) TimeTillNextWindow() time.Duration {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return time.Until(fw.resetAt)
}

// make a request
//
// will return an error if the limit has been reached
func (fw *FixedWindow) Request() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.checkAndDoReset()
	if fw.limitReached() {
		return fmt.Errorf("limit: %d has been reached for this window", fw.MaxRequests)
	}
	fw.count++
	return nil
}

// make a request
//
// will wait until the request can be completed, or the context times out.
func (fw *FixedWindow) WaitRequest(ctx context.Context) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	// becuase selects are non-deterministic, we need to manually check if the context is done on arrival
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled: %w", ctx.Err())
	}
	select {
	case <-time.After(fw.timeUntilMakeRequest()):
		fw.checkAndDoReset()
		if fw.limitReached() {
			// hopefully this will never trigger because the count has just been reset
			// but theoretically it would happen such that enough other things made requests
			return fmt.Errorf("limit: %d has been reached for this window", fw.MaxRequests)
		}
		fw.count++
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	}
}
