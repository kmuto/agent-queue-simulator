package main

import (
	"errors"
	"fmt"
	"time"
)

type loopState uint8

const (
	loopStateFirst loopState = iota
	loopStateDefault
	loopStateQueued
	loopStateHadError
	loopStateTerminating
)

type postValue struct {
	values   []int64
	retryCnt int
}

func newPostValue(values []int64) *postValue {
	return &postValue{values, 0}
}

func postHostMetricValuesWithRetry(postValues []int64, inDown bool) error {
	if inDown {
		return errors.New("IN DOWN")
	}
	return nil
}

// 障害時間配列を作る。30秒ごとにダウンタイムのときはtrueにする
func makeDownTimeMap() map[int64]bool {
	downTimeMap := make(map[int64]bool)
	from_t := time.Date(2023, 7, 28, 10, 15, 0, 0, time.Local).Unix()
	to_t := time.Date(2023, 7, 28, 10, 30, 0, 0, time.Local).Unix()
	for t := from_t; t < to_t; t += 30 {
		downTimeMap[t] = true
	}
	return downTimeMap
}

func log(t int64, message string) {
	fmt.Println(fmt.Sprintf("%v", time.Unix(t, 0)), ": ", message)
}

// 30秒単位での時間を黙々と返す
func ticks(ticksChan chan<- int64, from time.Time, to time.Time, termCh chan<- struct{}) {
	from_t := from.Unix()
	to_t := to.Unix()
	for t := from_t; t < to_t; t += 60 {
		ticksChan <- t
		ticksChan <- t + 30
	}
	termCh <- struct{}{}
}

func main() {
	from := time.Date(2023, 7, 28, 10, 0, 0, 0, time.Local)
	to := time.Date(2023, 7, 28, 11, 0, 0, 0, time.Local)
	ticksCh := make(chan int64)
	downTimeMap := makeDownTimeMap()
	inDown := false
	nowTime := int64(0)

	postQueue := make(chan *postValue, 3)
	termCh := make(chan struct{})

	go ticks(ticksCh, from, to, termCh)
	lState := loopStateFirst

	for {
		select {
		case t := <-ticksCh:
			nowTime = t
			if downTimeMap[t] {
				// fmt.Printf("!DOWN %v\n", time.Unix(t, 0))
				inDown = true
			} else {
				inDown = false
			}
			if t%60 == 0 {
				creatingValues := []int64{t}
				postQueue <- newPostValue(creatingValues) // これは障害だろうがやる
			}
		case v := <-postQueue:
			origPostValues := [](*postValue){v}
			if len(postQueue) > 0 {
				fmt.Println("BULK")
				nextValues := <-postQueue
				origPostValues = append(origPostValues, nextValues)
			}
			// delay

			if lState != loopStateTerminating {
				if len(postQueue) > 0 {
					lState = loopStateQueued
				} else {
					lState = loopStateDefault
				}
			}

			var postValues []int64
			for _, v := range origPostValues {
				postValues = append(postValues, v.values...)
			}
			err := postHostMetricValuesWithRetry(postValues, inDown)
			if err != nil {
				if lState != loopStateTerminating {
					lState = loopStateHadError
				}
			}

			log(nowTime, fmt.Sprintf("%v", v))
		case <-termCh:
			fmt.Println("EXIT")
			return
		}
	}
	//go ticks(timerPost, from, to)
	/*for i := range timerReceive {
		if i%60 == 0 {
			if downTimeMap[i] {
				fmt.Println("!!", time.Unix(i, 0))
			}
			// 投稿
			//			queue <- fmt.Sprintf("%v", time.Unix(i, 0))
		}
	}*/

	//fmt.Println(len(queue))

	/*	select {
		case <-ticksChan:
			fmt.Println("HAITTA")
		default:
			fmt.Println("NO DATA")
		}
		time.Sleep(60 * time.Second) */
	//mainLoop(ticksChan, myQueue)
	//wg.Wait()*/
}
