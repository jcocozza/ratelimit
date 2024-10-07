package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jcocozza/ratelimit"
)

const (
	Short         = 300
	ShortDuration = time.Duration(1 * time.Second)
	Long          = 1000
	LongDuration  = time.Duration(1 * time.Minute)

	throttleShort = 150
	throttleLong = 500
)

type data struct {
	id string
}

type service2 struct {
	work     chan data
	limShort *ratelimit.FixedWindow
	limLong  *ratelimit.FixedWindow
}

func (s *service2) makeRequest(ctx context.Context) error {
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

func (s *service2) produceWork() {
	for i := 0; i < 10000; i++ {
		s.work <- data{id: fmt.Sprintf("uuid_%d", i)}
	}
}

func (s *service2) processWorkConcurrent() {
	const numWorkers = 5
	ctx := context.Background()
	worker := func() {
		for {
			select {
			case wrk, ok := <-s.work:
				if !ok {
					fmt.Println("not okay, returning")
					return
				}
				err := s.makeRequest(ctx)
				if err != nil {
					fmt.Println("error: " + err.Error())
				}
				fmt.Printf("processing: %v | [%s][%s]\n", wrk, s.limShort, s.limLong)
			}
		}
	}
	for i := 0; i < numWorkers; i++ {
		go worker()
	}
	// block
	select {}
}

func main2() {
	serv := &service2{
		work:     make(chan data),
		limShort: ratelimit.NewFixedWindow(ShortDuration, Short),
		limLong:  ratelimit.NewFixedWindow(LongDuration, Long),
	}
	go serv.processWorkConcurrent()
	go serv.produceWork()
	//block
	select {}
}
