package main

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"time"

	strftime "github.com/itchyny/timefmt-go"
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
		fmt.Println(strftime.Format(time.Unix(t, 0), "%H:%M:%S"), ",", status, ", QLEN", len)
	} else {
		message := fmt.Sprintf("%s (RETRY %d)", strftime.Format(time.Unix(postValue.values[0], 0), "%H:%M"), postValue.retryCnt)
		fmt.Println(strftime.Format(time.Unix(t, 0), "%H:%M:%S"), ",", status, ",", message, ": QLEN", len)
	}
}

// 30秒単位での時間を黙々と返す
func ticks(ticksChan chan<- int64, from time.Time, to time.Time, termCh chan<- struct{}) {
	from_t := from.Unix()
	to_t := to.Unix()
	for t := from_t; t < to_t; t += 60 {
		ticksChan <- t
		time.Sleep(10 * time.Millisecond)
		ticksChan <- t + 30
		time.Sleep(10 * time.Millisecond)
	}
	termCh <- struct{}{}
}

func delayByHost(host string) int {
	s := sha1.Sum([]byte(host))
	return int(s[len(s)-1]) % int(60)
}

var nowTime int64

func main() {
	from := time.Date(2023, 7, 28, 3, 33, 0, 0, time.Local)
	to := time.Date(2023, 7, 28, 18, 0, 0, 0, time.Local)
	ticksCh := make(chan int64)
	downTimeMap := makeDownTimeMap()
	inDown := false
	nowTime = int64(0)

	postMetricsDequeueDelaySeconds := 30
	postMetricsRetryDelaySeconds := 60
	postMetricsRetryMax := 60
	postMetricsBufferSize := 6 * 60

	postDelaySeconds := delayByHost("AAAAAAAA") // ホストIDから生成する0〜59のやつ

	postQueue := make(chan *postValue, postMetricsBufferSize)
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
				if len(postQueue) < postMetricsBufferSize { // FIXME:本番ではこれはなくてうまく処理できている。goわからん
					postQueue <- newPostValue(creatingValues) // これは障害だろうが動き続けている
				} else {
					// 本当は捨てずに待機になるはず
					log(nowTime, "FLOOD", newPostValue(creatingValues), len(postQueue))
				}
			}
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
				delaySeconds = postMetricsDequeueDelaySeconds // postMetricsDequeueDelaySeconds = 30
			case loopStateHadError:
				delaySeconds = postMetricsRetryDelaySeconds // postMetricsRetryDelaySeconds = 60
			case loopStateTerminating:
				delaySeconds = 1 // 終了時 = 1
			default:
				elapsedSeconds := int(nowTime % int64(postDelaySeconds))
				if postDelaySeconds > elapsedSeconds {
					delaySeconds = postDelaySeconds - elapsedSeconds
				} // 今だと3,6,9
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
			// 繰り返しになってだいぶ嫌め。もう1つチャネルが必要なのかな

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
					if len(postQueue) < postMetricsBufferSize { // FIXME:これも本来いらない
						postQueue <- newPostValue(creatingValues) // これは障害だろうが動き続けている
					} else {
						// 本当は捨てずに待機になるはず
						log(nowTime, "FLOOD", newPostValue(creatingValues), len(postQueue))
					}
				}
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
					// log(nowTime, "FAIL", newPostValue(postValues), len(postQueue))
				}
				go func() {
					for _, v = range origPostValues {
						v.retryCnt++
						if v.retryCnt > postMetricsRetryMax { // postMetricsRetryMax=60
							log(nowTime, "LOST", v, len(postQueue))
							continue
						}
						postQueue <- v
						log(nowTime, "REQUEUE", v, len(postQueue))
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
