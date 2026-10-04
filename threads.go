package jxl

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

const minRowsPerWorker = 16

func numThreads(n int) int {
	if n <= 0 {
		return runtime.GOMAXPROCS(0)
	}

	return n
}

func overRows(n, rows int, fn func(y0, y1 int)) {
	n = min(n, rows/minRowsPerWorker)

	if n <= 1 {
		fn(0, rows)

		return
	}

	chunk := divCeil(rows, n)

	var wg sync.WaitGroup
	panics := make([]any, divCeil(rows, chunk))

	for y0 := 0; y0 < rows; y0 += chunk {
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					panics[y0/chunk] = p
				}
			}()

			fn(y0, min(y0+chunk, rows))
		}()
	}

	wg.Wait()
	for _, p := range panics {
		if p != nil {
			panic(p)
		}
	}
}

func safeTask(fn func(worker, task int) error, worker, task int) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: decoder worker panic", ErrInvalid)
		}
	}()
	return fn(worker, task)
}

func overTasks(n, tasks int, fn func(worker, task int) error) error {
	if n <= 1 || tasks <= 1 {
		for i := range tasks {
			if err := safeTask(fn, 0, i); err != nil {
				return err
			}
		}

		return nil
	}

	n = min(n, tasks)

	errs := make([]error, tasks)
	next := make(chan int, tasks)

	for i := range tasks {
		next <- i
	}

	close(next)

	var (
		wg  sync.WaitGroup
		bad atomic.Bool
	)

	for w := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range next {
				if bad.Load() {
					return
				}

				if errs[i] = safeTask(fn, w, i); errs[i] != nil {
					bad.Store(true)

					return
				}
			}
		}()
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}
