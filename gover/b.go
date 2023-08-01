package main

import (
	"context"
	"fmt"
	"time"
)

func isDownTime(t int64) bool {
	from_t := time.Date(2023, 7, 28, 03, 35, 0, 0, time.Local).Unix()
	to_t := time.Date(2023, 7, 28, 13, 45, 0, 0, time.Local).Unix()
	if t >= from_t && t < to_t {
		return true
	}
	return false
}

func tick(result chan int64, done chan struct{}) {
	from := time.Date(2023, 7, 28, 3, 33, 0, 0, time.Local)
	to := time.Date(2023, 7, 28, 18, 0, 0, 0, time.Local)
	from_t := from.Unix()
	to_t := to.Unix()

	for t := from_t; t < to_t; t++ {
		result <- t
	}
	done <- struct{}{}
}

type MetricsResult2 struct {
	Created time.Time
	Values  int64
}

func Watch2(ctx context.Context, alldone chan struct{}) chan *MetricsResult2 {
	metricsResult := make(chan *MetricsResult2)
	ticktuck := make(chan int64, 20)
	done := make(chan struct{})

	go tick(ticktuck, done)
	ticker := make(chan int64, 20)

	go func() {
		for {
			select {
			case <-done:
				alldone <- struct{}{}
				return
			case t := <-ticktuck:
				//if t%int64(60) == 0 {
				select {
				case ticker <- t:
					fmt.Println(t)
				default:
				}
				//}
				// fmt.Println("tick", t)
			}
		}
	}()

	go func() {
		sem := make(chan struct{}, 3)
		for tickedTime := range ticker {
			ti := tickedTime
			sem <- struct{}{}
			go func() {
				metricsResult <- &MetricsResult2{
					Created: time.Unix(ti, 0),
					Values:  ti}
				<-sem
			}()
		}
	}()
	return metricsResult
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	alldone := make(chan struct{}, 1)

	metricsResult := Watch2(ctx, alldone)
	fmt.Println(metricsResult)

	select {
	case <-alldone:
		fmt.Println("FINISHED")
		break
	}
	/*
		result := make(chan int64, 2000)
		done := make(chan struct{})
		go tick(result, done)
		for {
			select {
			case <-done:
				return
			case t := <-result:
				fmt.Println()
			default:
			}
		} */
}
