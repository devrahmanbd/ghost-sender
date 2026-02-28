package notification

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrQueueClosed = errors.New("queue is closed")
	ErrQueueEmpty  = errors.New("queue is empty")
)

type NotificationQueue struct {
	mu          sync.RWMutex
	items       []*QueueItem
	maxSize     int
	closed      bool
	notEmpty    *sync.Cond
	metrics     *QueueMetrics
	prioritized bool
}

type QueueItem struct {
	Notification *Notification
	EnqueuedAt   time.Time
	Priority     int
	Retries      int
}

type QueueMetrics struct {
	mu              sync.RWMutex
	TotalEnqueued   int64
	TotalDequeued   int64
	TotalDropped    int64
	CurrentSize     int64
	MaxSizeReached  int64
	AverageWaitTime time.Duration
	TotalWaitTime   time.Duration
}

type QueueConfig struct {
	MaxSize     int
	Prioritized bool
	DropPolicy  DropPolicy
}

type DropPolicy string

const (
	DropPolicyOldest DropPolicy = "oldest"
	DropPolicyLowest DropPolicy = "lowest_priority"
	DropPolicyNewest DropPolicy = "newest"
)

func NewNotificationQueue(maxSize int) *NotificationQueue {
	nq := &NotificationQueue{
		items:       make([]*QueueItem, 0, maxSize),
		maxSize:     maxSize,
		closed:      false,
		prioritized: true,
		metrics: &QueueMetrics{
			CurrentSize: 0,
		},
	}
	nq.notEmpty = sync.NewCond(&nq.mu)

	return nq
}

func NewNotificationQueueWithConfig(config *QueueConfig) *NotificationQueue {
	if config == nil {
		config = DefaultQueueConfig()
	}

	nq := &NotificationQueue{
		items:       make([]*QueueItem, 0, config.MaxSize),
		maxSize:     config.MaxSize,
		closed:      false,
		prioritized: config.Prioritized,
		metrics: &QueueMetrics{
			CurrentSize: 0,
		},
	}
	nq.notEmpty = sync.NewCond(&nq.mu)

	return nq
}

func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		MaxSize:     1000,
		Prioritized: true,
		DropPolicy:  DropPolicyOldest,
	}
}

func (nq *NotificationQueue) Enqueue(notification *Notification) error {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	if nq.closed {
		return ErrQueueClosed
	}

	item := &QueueItem{
		Notification: notification,
		EnqueuedAt:   time.Now(),
		Priority:     notification.Priority,
		Retries:      notification.Retries,
	}

	if len(nq.items) >= nq.maxSize {
		nq.recordDropped()
		return ErrQueueFull
	}

	nq.items = append(nq.items, item)

	if nq.prioritized {
		nq.sortByPriority()
	}

	nq.recordEnqueued()
	nq.notEmpty.Signal()

	return nil
}

func (nq *NotificationQueue) EnqueueBatch(notifications []*Notification) error {
	for _, notification := range notifications {
		if err := nq.Enqueue(notification); err != nil {
			return err
		}
	}
	return nil
}

func (nq *NotificationQueue) Dequeue(ctx context.Context, timeout time.Duration) (*Notification, error) {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	deadline := time.Now().Add(timeout)

	for len(nq.items) == 0 && !nq.closed {
		if timeout > 0 {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, context.DeadlineExceeded
			}

			timer := time.AfterFunc(remaining, func() {
				nq.notEmpty.Broadcast()
			})
			nq.notEmpty.Wait()
			timer.Stop()

			if time.Now().After(deadline) {
				return nil, context.DeadlineExceeded
			}
		} else {
			nq.notEmpty.Wait()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	if nq.closed && len(nq.items) == 0 {
		return nil, ErrQueueClosed
	}

	if len(nq.items) == 0 {
		return nil, ErrQueueEmpty
	}

	item := nq.items[0]
	nq.items = nq.items[1:]

	waitTime := time.Since(item.EnqueuedAt)
	nq.recordDequeued(waitTime)

	return item.Notification, nil
}

func (nq *NotificationQueue) TryDequeue() (*Notification, error) {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	if nq.closed {
		return nil, ErrQueueClosed
	}

	if len(nq.items) == 0 {
		return nil, ErrQueueEmpty
	}

	item := nq.items[0]
	nq.items = nq.items[1:]

	waitTime := time.Since(item.EnqueuedAt)
	nq.recordDequeued(waitTime)

	return item.Notification, nil
}

func (nq *NotificationQueue) Peek() (*Notification, error) {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	if nq.closed {
		return nil, ErrQueueClosed
	}

	if len(nq.items) == 0 {
		return nil, ErrQueueEmpty
	}

	return nq.items[0].Notification, nil
}

func (nq *NotificationQueue) Size() int {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	return len(nq.items)
}

func (nq *NotificationQueue) IsEmpty() bool {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	return len(nq.items) == 0
}

func (nq *NotificationQueue) IsFull() bool {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	return len(nq.items) >= nq.maxSize
}

func (nq *NotificationQueue) IsClosed() bool {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	return nq.closed
}

func (nq *NotificationQueue) Close() error {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	if nq.closed {
		return errors.New("queue already closed")
	}

	nq.closed = true
	nq.notEmpty.Broadcast()

	return nil
}

func (nq *NotificationQueue) Clear() {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	nq.items = make([]*QueueItem, 0, nq.maxSize)
	nq.metrics.CurrentSize = 0
}

func (nq *NotificationQueue) Drain() []*Notification {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	notifications := make([]*Notification, len(nq.items))
	for i, item := range nq.items {
		notifications[i] = item.Notification
	}

	nq.items = make([]*QueueItem, 0, nq.maxSize)
	nq.metrics.CurrentSize = 0

	return notifications
}

func (nq *NotificationQueue) sortByPriority() {
	if len(nq.items) <= 1 {
		return
	}

	for i := 1; i < len(nq.items); i++ {
		key := nq.items[i]
		j := i - 1

		for j >= 0 && nq.items[j].Priority < key.Priority {
			nq.items[j+1] = nq.items[j]
			j--
		}
		nq.items[j+1] = key
	}
}

func (nq *NotificationQueue) recordEnqueued() {
	nq.metrics.mu.Lock()
	defer nq.metrics.mu.Unlock()

	nq.metrics.TotalEnqueued++
	nq.metrics.CurrentSize = int64(len(nq.items))

	if nq.metrics.CurrentSize > nq.metrics.MaxSizeReached {
		nq.metrics.MaxSizeReached = nq.metrics.CurrentSize
	}
}

func (nq *NotificationQueue) recordDequeued(waitTime time.Duration) {
	nq.metrics.mu.Lock()
	defer nq.metrics.mu.Unlock()

	nq.metrics.TotalDequeued++
	nq.metrics.CurrentSize = int64(len(nq.items))
	nq.metrics.TotalWaitTime += waitTime

	if nq.metrics.TotalDequeued > 0 {
		nq.metrics.AverageWaitTime = time.Duration(int64(nq.metrics.TotalWaitTime) / nq.metrics.TotalDequeued)
	}
}

func (nq *NotificationQueue) recordDropped() {
	nq.metrics.mu.Lock()
	defer nq.metrics.mu.Unlock()

	nq.metrics.TotalDropped++
}

func (nq *NotificationQueue) GetMetrics() *QueueMetrics {
	nq.metrics.mu.RLock()
	defer nq.metrics.mu.RUnlock()

	return &QueueMetrics{
		TotalEnqueued:   nq.metrics.TotalEnqueued,
		TotalDequeued:   nq.metrics.TotalDequeued,
		TotalDropped:    nq.metrics.TotalDropped,
		CurrentSize:     nq.metrics.CurrentSize,
		MaxSizeReached:  nq.metrics.MaxSizeReached,
		AverageWaitTime: nq.metrics.AverageWaitTime,
		TotalWaitTime:   nq.metrics.TotalWaitTime,
	}
}

func (nq *NotificationQueue) ResetMetrics() {
	nq.metrics.mu.Lock()
	defer nq.metrics.mu.Unlock()

	nq.metrics.TotalEnqueued = 0
	nq.metrics.TotalDequeued = 0
	nq.metrics.TotalDropped = 0
	nq.metrics.MaxSizeReached = nq.metrics.CurrentSize
	nq.metrics.AverageWaitTime = 0
	nq.metrics.TotalWaitTime = 0
}

func (nq *NotificationQueue) GetAll() []*Notification {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	notifications := make([]*Notification, len(nq.items))
	for i, item := range nq.items {
		notifications[i] = item.Notification
	}

	return notifications
}

func (nq *NotificationQueue) Contains(notificationID string) bool {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	for _, item := range nq.items {
		if item.Notification.ID == notificationID {
			return true
		}
	}

	return false
}

func (nq *NotificationQueue) Remove(notificationID string) bool {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	for i, item := range nq.items {
		if item.Notification.ID == notificationID {
			nq.items = append(nq.items[:i], nq.items[i+1:]...)
			nq.metrics.CurrentSize = int64(len(nq.items))
			return true
		}
	}

	return false
}

func (nq *NotificationQueue) Filter(predicate func(*Notification) bool) []*Notification {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	var filtered []*Notification
	for _, item := range nq.items {
		if predicate(item.Notification) {
			filtered = append(filtered, item.Notification)
		}
	}

	return filtered
}

func (nq *NotificationQueue) RemoveIf(predicate func(*Notification) bool) int {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	removed := 0
	newItems := make([]*QueueItem, 0, len(nq.items))

	for _, item := range nq.items {
		if !predicate(item.Notification) {
			newItems = append(newItems, item)
		} else {
			removed++
		}
	}

	nq.items = newItems
	nq.metrics.CurrentSize = int64(len(nq.items))

	return removed
}

func (nq *NotificationQueue) GetByLevel(level NotificationLevel) []*Notification {
	return nq.Filter(func(n *Notification) bool {
		return n.Level == level
	})
}

func (nq *NotificationQueue) GetByEvent(eventType EventType) []*Notification {
	return nq.Filter(func(n *Notification) bool {
		return n.Event == eventType
	})
}

func (nq *NotificationQueue) GetHighPriority(minPriority int) []*Notification {
	return nq.Filter(func(n *Notification) bool {
		return n.Priority >= minPriority
	})
}

func (nq *NotificationQueue) RemoveOlderThan(duration time.Duration) int {
	cutoff := time.Now().Add(-duration)

	nq.mu.Lock()
	defer nq.mu.Unlock()

	removed := 0
	newItems := make([]*QueueItem, 0, len(nq.items))

	for _, item := range nq.items {
		if item.EnqueuedAt.After(cutoff) {
			newItems = append(newItems, item)
		} else {
			removed++
		}
	}

	nq.items = newItems
	nq.metrics.CurrentSize = int64(len(nq.items))

	return removed
}

func (nq *NotificationQueue) Capacity() int {
	return nq.maxSize
}

func (nq *NotificationQueue) Available() int {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	return nq.maxSize - len(nq.items)
}

func (nq *NotificationQueue) UsagePercent() float64 {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	if nq.maxSize == 0 {
		return 0
	}

	return float64(len(nq.items)) / float64(nq.maxSize) * 100
}

func (nq *NotificationQueue) Wait() {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	for len(nq.items) > 0 && !nq.closed {
		nq.notEmpty.Wait()
	}
}

func (nq *NotificationQueue) WaitWithTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	nq.mu.Lock()
	defer nq.mu.Unlock()

	for len(nq.items) > 0 && !nq.closed {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}

		timer := time.AfterFunc(remaining, func() {
			nq.notEmpty.Broadcast()
		})
		nq.notEmpty.Wait()
		timer.Stop()

		if time.Now().After(deadline) {
			return false
		}
	}

	return true
}

