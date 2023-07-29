package main

import (
	"fmt"
	"time"
)

// 30秒単位での時間を黙々と返す
func ticks(ticksChan chan<- int64, from time.Time, to time.Time) {
	defer close(ticksChan)
	from_i := from.Unix()
	to_i := to.Unix()
	for i := from_i; i < to_i; i += 60 {
		ticksChan <- i
		ticksChan <- i + 30
	}
}

// 障害時間配列を作る
func makeDownTimeMap() map[int64]bool {
	downTimeMap := make(map[int64]bool)
	from := time.Date(2023, 7, 28, 10, 15, 0, 0, time.Local).Unix()
	to := time.Date(2023, 7, 28, 10, 30, 0, 0, time.Local).Unix()
	for i := from; i < to; i++ {
		downTimeMap[i] = true
	}
	return downTimeMap
}

func postMetric(queue chan<- string, tick int64) {
	queue <- fmt.Sprintf("%v", time.Unix(tick, 0))
}

func mainLoop(ticksChan <-chan int64, queue chan string) {
	downTimeMap := makeDownTimeMap()
	for i := range ticksChan {
		if i%60 == 0 {
			if downTimeMap[i] {
				fmt.Print("!!")
			}
			postMetric(queue, i)
		}
	}

	for data := range queue {
		fmt.Println(data)
	}
	close(queue)
}

func main() {
	from := time.Date(2023, 7, 28, 10, 0, 0, 0, time.Local)
	to := time.Date(2023, 7, 28, 11, 0, 0, 0, time.Local)
	ticksChan := make(chan int64)
	downTimeMap := makeDownTimeMap()

	go ticks(ticksChan, from, to)
	for i := range ticksChan {
		if i%60 == 0 {
			if downTimeMap[i] {
				fmt.Println("!!", time.Unix(i, 0))
			}
		}
	}

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
