package event

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	emitter := NewRootPolicyEventEmitter(context.TODO(), nil)
	assert.Equal(t, "0.0", emitter.currentVersion.String())
	assert.Equal(t, "0.0", emitter.lastSentVersion.String())
	assert.False(t, emitter.Emit())

	emitter.currentVersion.Incr()
	assert.Equal(t, "0.1", emitter.currentVersion.String())
	assert.Equal(t, "0.0", emitter.lastSentVersion.String())
	assert.True(t, emitter.Emit())

	emitter.PostSend()
	assert.Equal(t, "1.1", emitter.currentVersion.String())
	assert.Equal(t, "1.1", emitter.lastSentVersion.String())
	assert.False(t, emitter.Emit())

	emitter.currentVersion.Incr()
	assert.Equal(t, "1.2", emitter.currentVersion.String())
	assert.Equal(t, "1.1", emitter.lastSentVersion.String())
	assert.True(t, emitter.Emit())
}
