/*
This is a disgusting example of how I build the package to be used

Here this idea is relatively simple.
A long running thing(like a server), creates some work.
That work is send to a never-closed channel which has some workers watching the channel.
Once the workers see some work, they go and do it.

In the real world, maybe your workers need to call some api that has request limits attached to it.
So you have a service that keeps track of your request limits for you. Here, I imagine 2 limits at once (a short and long window).
But since you're "backfilling" you don't really care when these get done.
They can just sit around until you have some free requests.
(Hence the additional checks on a lower threshold)
*/
package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/jcocozza/ratelimit"
)

const (
	ReadLimitShort         = 300.0
	ReadLimitShortDuration = time.Duration(500 * time.Millisecond)
	ReadLimitLong          = 3000.0
	ReadLimitLongDuration  = time.Duration(20 * time.Second)
)

type deepfill struct {
	UserUUID     string
	ActivityUUID string
}

type service struct {
	deepUUIDBackfill chan deepfill
	limShort         *ratelimit.FixedWindow
	limLong          *ratelimit.FixedWindow
}

func (s *service) Backfill() error {
	userUUID := "foo_bar_baz"
	uuidList := []string{}
	for i := 1; i < 1000; i++ {
		uuidList = append(uuidList, fmt.Sprintf("uuid_%d", i))
	}
	go func() {
		for _, uuid := range uuidList {
			s.deepUUIDBackfill <- deepfill{UserUUID: userUUID, ActivityUUID: uuid}
		}
	}()
	return nil
}

func (s *service) checkRateLimits(ctx context.Context) error {
	fmt.Println("checking rates...")
	err := s.limLong.WaitRequest(ctx)
	if err != nil {
		return fmt.Errorf("failed long rate limits, %w", err)
	}
	err = s.limShort.WaitRequest(ctx)
	if err != nil {
		return fmt.Errorf("failed short rate limits, %w", err)
	}
	return nil
}

func (s *service) RemainingRequests() (int, int) {
	rrdaily := s.limLong.RequestsRemaining()
	if rrdaily == 0 {
		return 0, 0
	}
	rr15 := s.limShort.RequestsRemaining()
	return rr15, rrdaily
}

func (s *service) deepBackfillRunner() {
	const min15BackfillThreshold = 150
	const dailyBackfillThreshold = 1500
	worker := func() {
		for {
			select {
			case deepfillObj, ok := <-s.deepUUIDBackfill:
				fmt.Println("current processing: ", deepfillObj)
				if !ok {
					return
				}
				for {
					min15Remaining, dailyRemaining := s.RemainingRequests()
					if dailyRemaining > dailyBackfillThreshold && min15Remaining > min15BackfillThreshold {
						fmt.Printf("processing allowed: short: %d, long: %d\n", min15Remaining, dailyRemaining)
						ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
						err := s.checkRateLimits(ctx)
						if err != nil {
							fmt.Println("error: " + err.Error())
						}
						cancel()
						break
					} else if dailyRemaining <= dailyBackfillThreshold {
						// if we exceeded the daily limit then just go to sleep till the next day
						// no point in doing lots of checks...
						timeTillNextDay := s.limLong.TimeTillNextWindow()
						time.Sleep(timeTillNextDay)
					} else {
						// if we exceeded the 15 min request limit, just sleep till the next window
						timeTillNext15min := s.limShort.TimeTillNextWindow()
						time.Sleep(timeTillNext15min)
					}
				}
			default:
				time.Sleep(time.Second)
			}
		}
	}
	var wg sync.WaitGroup
	numWorkers := 5
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker()
	}
	wg.Wait() // just to ensure that the deepBackfillRunner never returns
}

func main() {
	serv := &service{
		deepUUIDBackfill: make(chan deepfill, 100),
		limShort:         ratelimit.NewFixedWindow(ReadLimitShortDuration, ReadLimitShort),
		limLong:          ratelimit.NewFixedWindow(ReadLimitLongDuration, ReadLimitLong),
	}
	go serv.deepBackfillRunner()
	go serv.Backfill()

	go func() {
		random := rand.Int()
		time.Sleep(time.Duration(random) * time.Second)
		go serv.Backfill()
	}()

	// just keep the program from ever returning
	select {}
}
