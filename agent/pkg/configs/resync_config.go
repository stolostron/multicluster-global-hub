package configs

import "sync"

type ResyncTypeQueue struct {
	mu    sync.Mutex
	queue []string
}

// GlobalResyncQueue is a ready-to-use, globally accessible queue.
var GlobalResyncQueue = &ResyncTypeQueue{}

// Add appends a type to the end of the queue
func (q *ResyncTypeQueue) Add(t string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, t)
}

// Pop removes and returns the first element in the queue. Returns empty string if empty.
func (q *ResyncTypeQueue) Pop() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return ""
	}
	t := q.queue[0]
	q.queue = q.queue[1:]
	return t
}
