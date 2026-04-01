package main

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	BaseURL           = "http://localhost:8080"
	NumWorkers        = 50
	RequestsPerWorker = 100
)

var (
	successCount int64
	errorCount   int64
)

func submitTransaction(workerID int) {
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < RequestsPerWorker; i++ {
		req, err := http.NewRequest("POST", BaseURL+"/tx/create", nil)
		if err != nil {
			atomic.AddInt64(&errorCount, 1)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			atomic.AddInt64(&errorCount, 1)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			atomic.AddInt64(&successCount, 1)
		} else {
			atomic.AddInt64(&errorCount, 1)
		}
	}
}

func fetchChainInfo() {
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < RequestsPerWorker; i++ {
		resp, err := client.Get(BaseURL + "/chain/info")
		if err != nil {
			atomic.AddInt64(&errorCount, 1)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			atomic.AddInt64(&successCount, 1)
		} else {
			atomic.AddInt64(&errorCount, 1)
		}
	}
}

func main() {
	fmt.Println("=== NogoChain Load Test ===")
	fmt.Printf("Workers: %d, Requests per worker: %d\n", NumWorkers, RequestsPerWorker)

	start := time.Now()

	var wg sync.WaitGroup

	for i := 0; i < NumWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				submitTransaction(id)
			} else {
				fetchChainInfo()
			}
		}(i)
	}

	wg.Wait()

	elapsed := time.Since(start)
	totalRequests := int64(NumWorkers * RequestsPerWorker)

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Duration: %v\n", elapsed)
	fmt.Printf("Total requests: %d\n", totalRequests)
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", errorCount)
	fmt.Printf("Requests/sec: %.2f\n", float64(totalRequests)/elapsed.Seconds())
}
