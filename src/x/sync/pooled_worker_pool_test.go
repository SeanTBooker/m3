// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package sync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPooledWorkerPoolGo(t *testing.T) {
	var count uint32

	p, err := NewPooledWorkerPool(testWorkerPoolSize, NewPooledWorkerPoolOptions())
	require.NoError(t, err)
	p.Init()

	var wg sync.WaitGroup
	for i := 0; i < testWorkerPoolSize*2; i++ {
		wg.Add(1)
		p.Go(func() {
			atomic.AddUint32(&count, 1)
			wg.Done()
		})
	}
	wg.Wait()

	require.Equal(t, uint32(testWorkerPoolSize*2), count)
}

func TestPooledWorkerPoolGoWithTimeout(t *testing.T) {
	var (
		workers = 2
		opts    = NewPooledWorkerPoolOptions().SetNumShards(int64(workers))
	)
	p, err := NewPooledWorkerPool(workers, opts)
	require.NoError(t, err)
	p.Init()

	pooledWorkerPool, ok := p.(*pooledWorkerPool)
	require.True(t, ok)

	// First fill up all the queues without blocking.
	var wg sync.WaitGroup
	wg.Add(1)

	// Enqueue workers * 2 since buffered channel will allow workers / shards
	// (which is 1, since 2 / 2 = 1) which means we need to enqueue two times
	// the workers.
	totalEnqueue := workers * 2
	now := time.Now()
	for i := 0; i < totalEnqueue; i++ {
		// Set now in such a way that independent shards are selected.
		shardNowSelect := now.
			Truncate(time.Duration(totalEnqueue) * time.Nanosecond).
			Add(time.Duration(i) * time.Nanosecond)
		pooledWorkerPool.nowFn = func() time.Time {
			return shardNowSelect
		}

		result := p.GoWithTimeout(func() {
			wg.Wait()
		}, 100*time.Millisecond)
		assert.True(t, result)
	}

	// Restore the now fn.
	pooledWorkerPool.nowFn = time.Now

	// Now ensure all further enqueues time out.
	for i := 0; i < workers; i++ {
		result := p.GoWithTimeout(func() {
			wg.Wait()
		}, 100*time.Millisecond)
		assert.False(t, result)
	}

	// Release goroutines.
	wg.Done()
}

func TestPooledWorkerPoolGrowOnDemand(t *testing.T) {
	var count uint32

	p, err := NewPooledWorkerPool(
		1,
		NewPooledWorkerPoolOptions().
			SetGrowOnDemand(true))
	require.NoError(t, err)
	p.Init()

	var (
		wg       sync.WaitGroup
		numIters = testWorkerPoolSize * 2
		doneCh   = make(chan struct{})
	)
	wg.Add(numIters)

	for i := 0; i < numIters; i++ {
		// IfGoOrGrow did not allocate new goroutines then
		// this test would never complete this loop as the
		// anonymous Work function below would not complete
		// and would block further iterations.
		p.Go(func() {
			atomic.AddUint32(&count, 1)
			wg.Done()
			<-doneCh
		})
	}
	close(doneCh)
	wg.Wait()

	require.Equal(t, uint32(numIters), count)
}

func TestPooledWorkerPoolGoOrGrowKillWorker(t *testing.T) {
	var count uint32

	p, err := NewPooledWorkerPool(
		1,
		NewPooledWorkerPoolOptions().
			SetGrowOnDemand(true).
			SetKillWorkerProbability(1.0))
	require.NoError(t, err)
	p.Init()

	var (
		wg       sync.WaitGroup
		numIters = testWorkerPoolSize * 2
		doneCh   = make(chan struct{})
	)
	wg.Add(numIters)

	for i := 0; i < numIters; i++ {
		// IfGoOrGrow did not allocate new goroutines then
		// this test would never complete this loop as the
		// anonymous Work function below would not complete
		// and would block further iterations.
		p.Go(func() {
			atomic.AddUint32(&count, 1)
			wg.Done()
			<-doneCh
		})
	}
	close(doneCh)
	wg.Wait()

	require.Equal(t, uint32(numIters), count)
}

func TestPooledWorkerPoolGoKillWorker(t *testing.T) {
	var count uint32

	p, err := NewPooledWorkerPool(
		testWorkerPoolSize,
		NewPooledWorkerPoolOptions().
			SetKillWorkerProbability(1.0))
	require.NoError(t, err)
	p.Init()

	var wg sync.WaitGroup
	for i := 0; i < testWorkerPoolSize*2; i++ {
		wg.Add(1)
		p.Go(func() {
			atomic.AddUint32(&count, 1)
			wg.Done()
		})
	}
	wg.Wait()

	require.Equal(t, uint32(testWorkerPoolSize*2), count)
}

func TestPooledWorkerPoolSizeTooSmall(t *testing.T) {
	_, err := NewPooledWorkerPool(0, NewPooledWorkerPoolOptions())
	require.Error(t, err)
}
