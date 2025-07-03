package configs

import (
	"testing"
)

func TestResyncTypeQueue_Basic(t *testing.T) {
	queue := &ResyncTypeQueue{}

	// Test empty queue
	if queue.Pop() != "" {
		t.Error("Empty queue should return empty string")
	}

	// Test add and pop
	queue.Add("test1")
	queue.Add("test2")

	if queue.Pop() != "test1" {
		t.Error("Should return first item")
	}
	if queue.Pop() != "test2" {
		t.Error("Should return second item")
	}
	if queue.Pop() != "" {
		t.Error("Empty queue should return empty string")
	}
}

func TestGlobalResyncQueue(t *testing.T) {
	if GlobalResyncQueue == nil {
		t.Error("GlobalResyncQueue should not be nil")
	}

	GlobalResyncQueue.Add("test")
	if GlobalResyncQueue.Pop() != "test" {
		t.Error("Global queue should work")
	}
}
