package sender

import (
	"container/heap"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"fmt"
	"email-campaign-system/pkg/logger"
)

var (
	ErrQueueFull   = errors.New("queue is full")
	ErrQueueEmpty  = errors.New("queue is empty")
	ErrQueueClosed = errors.New("queue is closed")
)

type Queue struct {
	items         *PriorityQueue
	config        QueueConfig
	log           logger.Logger
	mu            sync.RWMutex
	notEmpty      *sync.Cond
	notFull       *sync.Cond
	closed        atomic.Bool
	metrics       *QueueMetrics
}

type QueueConfig struct {
	MaxSize         int
	PriorityEnabled bool
	BlockOnFull     bool
	BlockOnEmpty    bool
	DrainTimeout    time.Duration
}

type QueueItem struct {
	Data      interface{}
	Priority  int
	EnqueueAt time.Time
	Index     int
}

type PriorityQueue []*QueueItem

type QueueMetrics struct {
	TotalEnqueued   int64
	TotalDequeued   int64
	TotalDropped    int64
	CurrentSize     int64
	PeakSize        int64
	AverageWaitTime time.Duration
	TotalWaitTime   time.Duration
	LastEnqueueAt   time.Time
	LastDequeueAt   time.Time
	mu              sync.RWMutex
}

const (
	PriorityHigh   = 100
	PriorityNormal = 50
	PriorityLow    = 10
)

func NewQueue(config QueueConfig, log logger.Logger) *Queue {
	if config.MaxSize <= 0 {
		config.MaxSize = 1000
	}
	if config.DrainTimeout <= 0 {
		config.DrainTimeout = 30 * time.Second
	}

	q := &Queue{
		items:   &PriorityQueue{},
		config:  config,
		log:     log,
		metrics: &QueueMetrics{},
	}

	heap.Init(q.items)

	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)

	return q
}

func (q *Queue) Enqueue(item interface{}) error {
	return q.EnqueueWithPriority(item, PriorityNormal)
}

func (q *Queue) EnqueueWithPriority(item interface{}, priority int) error {
	if q.closed.Load() {
		return ErrQueueClosed
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for q.items.Len() >= q.config.MaxSize {
		if !q.config.BlockOnFull {
			atomic.AddInt64(&q.metrics.TotalDropped, 1)
			return ErrQueueFull
		}

		if q.closed.Load() {
			return ErrQueueClosed
		}

		q.notFull.Wait()
	}

	if q.closed.Load() {
		return ErrQueueClosed
	}

	queueItem := &QueueItem{
		Data:      item,
		Priority:  priority,
		EnqueueAt: time.Now(),
	}

	heap.Push(q.items, queueItem)

	currentSize := int64(q.items.Len())
	atomic.StoreInt64(&q.metrics.CurrentSize, currentSize)

	if currentSize > atomic.LoadInt64(&q.metrics.PeakSize) {
		atomic.StoreInt64(&q.metrics.PeakSize, currentSize)
	}

	atomic.AddInt64(&q.metrics.TotalEnqueued, 1)

	q.metrics.mu.Lock()
	q.metrics.LastEnqueueAt = time.Now()
	q.metrics.mu.Unlock()

	q.notEmpty.Signal()

	return nil
}

func (q *Queue) Dequeue() (interface{}, error) {
	if q.closed.Load() && q.IsEmpty() {
		return nil, ErrQueueClosed
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for q.items.Len() == 0 {
		if !q.config.BlockOnEmpty {
			return nil, ErrQueueEmpty
		}

		if q.closed.Load() {
			return nil, ErrQueueClosed
		}

		q.notEmpty.Wait()

		if q.closed.Load() && q.items.Len() == 0 {
			return nil, ErrQueueClosed
		}
	}

	queueItem := heap.Pop(q.items).(*QueueItem)

	currentSize := int64(q.items.Len())
	atomic.StoreInt64(&q.metrics.CurrentSize, currentSize)

	atomic.AddInt64(&q.metrics.TotalDequeued, 1)

	waitTime := time.Since(queueItem.EnqueueAt)
	q.metrics.mu.Lock()
	q.metrics.TotalWaitTime += waitTime
	q.metrics.LastDequeueAt = time.Now()

	totalDequeued := atomic.LoadInt64(&q.metrics.TotalDequeued)
	if totalDequeued > 0 {
		q.metrics.AverageWaitTime = time.Duration(
			int64(q.metrics.TotalWaitTime) / totalDequeued,
		)
	}
	q.metrics.mu.Unlock()

	q.notFull.Signal()

	return queueItem.Data, nil
}

func (q *Queue) TryDequeue() (interface{}, bool) {
	if q.closed.Load() && q.IsEmpty() {
		return nil, false
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.items.Len() == 0 {
		return nil, false
	}

	queueItem := heap.Pop(q.items).(*QueueItem)

	currentSize := int64(q.items.Len())
	atomic.StoreInt64(&q.metrics.CurrentSize, currentSize)

	atomic.AddInt64(&q.metrics.TotalDequeued, 1)

	q.metrics.mu.Lock()
	q.metrics.LastDequeueAt = time.Now()
	q.metrics.mu.Unlock()

	q.notFull.Signal()

	return queueItem.Data, true
}

func (q *Queue) Peek() (interface{}, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.items.Len() == 0 {
		return nil, ErrQueueEmpty
	}

	return (*q.items)[0].Data, nil
}

func (q *Queue) Size() int {
	return int(atomic.LoadInt64(&q.metrics.CurrentSize))
}

func (q *Queue) IsEmpty() bool {
	return q.Size() == 0
}

func (q *Queue) IsFull() bool {
	return q.Size() >= q.config.MaxSize
}

func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = &PriorityQueue{}
	heap.Init(q.items)

	atomic.StoreInt64(&q.metrics.CurrentSize, 0)

	q.notFull.Broadcast()
}

func (q *Queue) Close() {
	if q.closed.Swap(true) {
		return
	}

	q.mu.Lock()
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
	q.mu.Unlock()

	q.log.Info(fmt.Sprintf("queue closed: remaining_items=%d, total_enqueued=%d, total_dequeued=%d",
		q.Size(),
		atomic.LoadInt64(&q.metrics.TotalEnqueued),
		atomic.LoadInt64(&q.metrics.TotalDequeued),
	))
}

func (q *Queue) Drain() []interface{} {
	q.mu.Lock()
	defer q.mu.Unlock()

	drained := make([]interface{}, 0, q.items.Len())

	for q.items.Len() > 0 {
		queueItem := heap.Pop(q.items).(*QueueItem)
		drained = append(drained, queueItem.Data)
	}

	atomic.StoreInt64(&q.metrics.CurrentSize, 0)

	return drained
}

func (q *Queue) GetMetrics() *QueueMetrics {
	q.metrics.mu.RLock()
	defer q.metrics.mu.RUnlock()

	return &QueueMetrics{
		TotalEnqueued:   atomic.LoadInt64(&q.metrics.TotalEnqueued),
		TotalDequeued:   atomic.LoadInt64(&q.metrics.TotalDequeued),
		TotalDropped:    atomic.LoadInt64(&q.metrics.TotalDropped),
		CurrentSize:     atomic.LoadInt64(&q.metrics.CurrentSize),
		PeakSize:        atomic.LoadInt64(&q.metrics.PeakSize),
		AverageWaitTime: q.metrics.AverageWaitTime,
		TotalWaitTime:   q.metrics.TotalWaitTime,
		LastEnqueueAt:   q.metrics.LastEnqueueAt,
		LastDequeueAt:   q.metrics.LastDequeueAt,
	}
}

func (q *Queue) IsClosed() bool {
	return q.closed.Load()
}

func (pq PriorityQueue) Len() int {
	return len(pq)
}

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Priority > pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueueItem)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) Update(item *QueueItem, priority int) {
	item.Priority = priority
	heap.Fix(pq, item.Index)
}
