package utils

import (
	"sync"
)

// RingQueue 是一个固定容量的泛型环形队列。
type RingQueue[T any] struct {
	mu       sync.Mutex
	data     []T
	head     int // 指向最旧的元素
	tail     int // 指向下一个可写入的位置
	count    int // 当前元素数量
	capacity int
}

// NewRingQueue 创建一个指定容量的环形队列
func NewRingQueue[T any](capacity int) *RingQueue[T] {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingQueue[T]{
		data:     make([]T, capacity),
		capacity: capacity,
		head:     0,
		tail:     0,
		count:    0,
	}
}

// Push 向队列尾部添加元素。
// 如果队列已满，最旧的元素将被覆盖。
func (q *RingQueue[T]) Push(item T) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.data[q.tail] = item
	q.tail = (q.tail + 1) % q.capacity

	if q.count < q.capacity {
		q.count++
	} else {
		q.head = (q.head + 1) % q.capacity
	}
}

// Pop 从队列头部取出并移除一个元素。
func (q *RingQueue[T]) Pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.count == 0 {
		var zero T
		return zero, false
	}

	item := q.data[q.head]
	var zero T
	q.data[q.head] = zero

	q.head = (q.head + 1) % q.capacity
	q.count--

	return item, true
}

// Peek 查看队列头部的元素，但不移除。
func (q *RingQueue[T]) Peek() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.count == 0 {
		var zero T
		return zero, false
	}

	return q.data[q.head], true
}

// GetAll 获取队列中所有有效数据。
func (q *RingQueue[T]) GetAll() []T {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.count == 0 {
		return nil
	}

	result := make([]T, q.count)
	for i := 0; i < q.count; i++ {
		idx := (q.head + i) % q.capacity
		result[i] = q.data[idx]
	}
	return result
}

// ClearAndFetch 获取所有数据并清空队列
func (q *RingQueue[T]) ClearAndFetch() []T {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.count == 0 {
		return nil
	}

	result := make([]T, q.count)
	for i := 0; i < q.count; i++ {
		idx := (q.head + i) % q.capacity
		result[i] = q.data[idx]
	}

	// 重置状态
	q.head = 0
	q.tail = 0
	q.count = 0

	// 帮助 GC
	for i := range q.data {
		var zero T
		q.data[i] = zero
	}

	return result
}

// Len 返回当前队列中的元素数量
func (q *RingQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.count
}

// Cap 返回队列容量
func (q *RingQueue[T]) Cap() int {
	return q.capacity
}
