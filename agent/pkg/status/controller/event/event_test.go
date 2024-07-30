package event

import (
	"context"
	"fmt"
	"testing"

	lru "github.com/hashicorp/golang-lru"
	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	emitter := NewLocalRootPolicyEmitter(context.TODO(), nil, "event")
	assert.Equal(t, "0.0", emitter.currentVersion.String())
	assert.Equal(t, "0.0", emitter.lastSentVersion.String())
	assert.False(t, emitter.ShouldSend())

	emitter.currentVersion.Incr()
	assert.Equal(t, "0.1", emitter.currentVersion.String())
	assert.Equal(t, "0.0", emitter.lastSentVersion.String())
	assert.True(t, emitter.ShouldSend())

	emitter.PostSend()
	assert.Equal(t, "1.1", emitter.currentVersion.String())
	assert.Equal(t, "1.1", emitter.lastSentVersion.String())
	assert.False(t, emitter.ShouldSend())

	emitter.currentVersion.Incr()
	assert.Equal(t, "1.2", emitter.currentVersion.String())
	assert.Equal(t, "1.1", emitter.lastSentVersion.String())
	assert.True(t, emitter.ShouldSend())
}

func TestLRU(t *testing.T) {
	l, _ := lru.New(5)
	for i := 0; i < 10; i++ {
		l.Add(fmt.Sprint(i), nil)
	}
	assert.Equal(t, 5, l.Len())

	ok := l.Contains("1")
	assert.False(t, ok)
	ok = l.Contains("4")
	assert.False(t, ok)

	ok = l.Contains("5")
	assert.True(t, ok)
	ok = l.Contains("6")
	assert.True(t, ok)
	ok = l.Contains("9")
	assert.True(t, ok)
}
