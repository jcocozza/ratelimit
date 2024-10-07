package ratelimit

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

// mostly for debugging
func (fw *FixedWindow) String() string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fmt.Sprintf("[Max Requests: %d, Interval: %s] %d", fw.MaxRequests, fw.Interval.String(), fw.count)
}

func (fw *FixedWindow) resetCount() {
	now := time.Now()
	fw.count = 0
	fw.resetAt = now.Add(fw.Interval)
}

func (fw *FixedWindow) checkAndDoReset() {
	if time.Now().After(fw.resetAt) {
		fw.resetCount()
	}
}

func (fw *FixedWindow) limitReached() bool {
	return fw.count >= fw.MaxRequests
}

// return the number of requests left in the window
func (fw *FixedWindow) RequestsRemaining() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.checkAndDoReset()
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
func (fw *FixedWindow) Request(opts ...RequestOption) error {
	options := createOptions(opts)
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.checkAndDoReset()
	if options.throttleLimit != nil && fw.count >= *options.throttleLimit {
		return fmt.Errorf("throttle limit: %d has been reached for this window", options.throttleLimit)
	}
	if fw.limitReached() {
		return fmt.Errorf("limit: %d has been reached for this window", fw.MaxRequests)
	}
	fw.count++
	return nil
}

// make a request
//
// will wait until the request can be completed, or the context times out.
func (fw *FixedWindow) WaitRequest(ctx context.Context, opts ...RequestOption) error {
	options := createOptions(opts)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled on arrival: %w", err)
	}
	for {
		fw.mu.Lock()
		fw.checkAndDoReset()
		if options.throttleLimit != nil {
			if fw.count < *options.throttleLimit && !fw.limitReached() {
				fw.count++
				fw.mu.Unlock()
				return nil
			}
		} else {
			if !fw.limitReached() {
				fw.count++
				fw.mu.Unlock()
				return nil
			}
		}
		waitTime := time.Until(fw.resetAt)
		fw.mu.Unlock()
		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		}
	}
}
