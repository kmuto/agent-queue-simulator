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
	from_t := time.Date(2023, 7, 28, 03, 35, 0, 0, time.Local).Unix()
	to_t := time.Date(2023, 7, 28, 13, 45, 0, 0, time.Local).Unix()
	for t := from_t; t < to_t; t += 30 {
		downTimeMap[t] = true
	}
	return downTimeMap
}

func log(t int64, status string, postValue *postValue, len int) {
	if postValue == nil {
		fmt.Println(fmt.Sprintf("%v", time.Unix(t, 0)), ":", status, ":", len)
	} else {
		message := fmt.Sprintf("%v (%d)", time.Unix(postValue.values[0], 0), postValue.retryCnt)
		fmt.Println(fmt.Sprintf("%v", time.Unix(t, 0)), ":", status, ":", message, ":", len)
	}
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
	from := time.Date(2023, 7, 28, 3, 33, 0, 0, time.Local)
	to := time.Date(2023, 7, 28, 18, 0, 0, 0, time.Local)
	ticksCh := make(chan int64)
	downTimeMap := makeDownTimeMap()
	inDown := false
	nowTime := int64(0)

	postQueue := make(chan *postValue, 360)
	termCh := make(chan struct{})

	go ticks(ticksCh, from, to, termCh)
	lState := loopStateFirst

	for {
		select {
		case t := <-ticksCh:
			nowTime = t
			if downTimeMap[t] {
				inDown = true
			} else {
				inDown = false
			}
			if t%60 == 0 {
				creatingValues := []int64{t}
				postQueue <- newPostValue(creatingValues) // これは障害だろうがやる
			}
			time.Sleep(10 * time.Millisecond)
		case v := <-postQueue:
			origPostValues := [](*postValue){v}
			if len(postQueue) > 0 {
				nextValues := <-postQueue
				origPostValues = append(origPostValues, nextValues)
			}
			// delay
			delaySeconds := 0
			switch lState {
			case loopStateFirst: // NOP
			case loopStateQueued:
				delaySeconds = 30 // postMetricsDequeueDelaySeconds = 30
			case loopStateHadError:
				delaySeconds = 60 // postMetricsRetryDelaySeconds = 60
			case loopStateTerminating:
				delaySeconds = 1 // 終了時 = 1
			default:
				delaySeconds = 1 // 環境により0〜59
			}
			targetTime := nowTime + int64(delaySeconds)

			if lState != loopStateTerminating {
				if len(postQueue) > 0 {
					lState = loopStateQueued
				} else {
					lState = loopStateDefault
				}
			}

			// FIXME: ここでnowTimeが次の時刻にいくまでdelaySecondsぶん待ちたいわけだが、これでいいのか…？

			select {
			case t := <-ticksCh:
				if t >= targetTime {
					break
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
					log(nowTime, "FAIL", newPostValue(postValues), len(postQueue))
				}
				go func() {
					for _, v = range origPostValues {
						v.retryCnt++
						if v.retryCnt > 60 { // postMetricsRetryMax=60
							log(nowTime, "LOST", v, len(postQueue))
							continue
						}
						postQueue <- v
					}
				}()
				continue
			}

			if lState == loopStateTerminating {
				if len(postQueue) <= 0 {
					return
				} else {
					// REMAIN FIXME:だめだな
					for v := range postQueue {
						log(nowTime, "REMAIN", v, len(postQueue))
					}
				}
			}

			log(nowTime, "POSTED", v, len(postQueue))
		case <-termCh:
			lState = loopStateTerminating
			if len(postQueue) <= 0 {
				fmt.Println("EXIT")
				return
			} else {
				for v := range postQueue {
					log(nowTime, "REMAIN", v, len(postQueue))
				}
			}
		}
	}
}
