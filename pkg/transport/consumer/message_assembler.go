// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
)

// messageChunk represents a chunk of a transport message.
type messageChunk struct {
	id        string
	timestamp time.Time
	offset    int
	size      int
	bytes     []byte
}

// messageChunksCollection holds a collection of chunks and maintains it until completion.
type messageChunksCollection struct {
	totalSize       int
	accumulatedSize int
	timestamp       time.Time
	chunks          map[int]*messageChunk
	lock            sync.Mutex
}

func newMessageChunksCollection(size int, timestamp time.Time) *messageChunksCollection {
	return &messageChunksCollection{
		totalSize:       size,
		accumulatedSize: 0,
		timestamp:       timestamp,
		chunks:          make(map[int]*messageChunk),
		lock:            sync.Mutex{},
	}
}

func (collection *messageChunksCollection) add(chunk *messageChunk) {
	collection.lock.Lock()
	defer collection.lock.Unlock()

	if chunk.offset+len(chunk.bytes) > collection.totalSize {
		return // chunk reaches out of message bounds
	}
	// don't add chunk to collection, if already exists.
	if _, found := collection.chunks[chunk.offset]; found {
		return
	}
	collection.chunks[chunk.offset] = chunk
	collection.accumulatedSize += len(chunk.bytes)
}

func (collection *messageChunksCollection) assemble() []byte {
	collection.lock.Lock()
	defer collection.lock.Unlock()
	buffer := make([]byte, collection.totalSize)
	for offset, chunk := range collection.chunks {
		copy(buffer[offset:], chunk.bytes)
		chunk.bytes = nil // faster GC
	}
	return buffer
}

type messageAssembler struct {
	log                logr.Logger
	chunkCollectionMap map[string]*messageChunksCollection
}

func newMessageAssembler() *messageAssembler {
	return &messageAssembler{
		log:                ctrl.Log.WithName("consumer-message-assembler"),
		chunkCollectionMap: make(map[string]*messageChunksCollection),
	}
}

// processChunk processes a message chunk and returns transport message if any got assembled, otherwise,nil.
func (assembler *messageAssembler) processChunk(chunk *messageChunk) *transport.Message {
	chunkCollection, found := assembler.chunkCollectionMap[chunk.id] // chunk.id: PlacementRule
	if !found || chunkCollection.timestamp.Before(chunk.timestamp) {
		// chunkCollection is not found or is hosting outdated chunks
		chunkCollection = newMessageChunksCollection(chunk.size, chunk.timestamp)
		assembler.chunkCollectionMap[chunk.id] = chunkCollection
	}

	if chunkCollection.timestamp.After(chunk.timestamp) {
		// chunk timestamp < collection got an outdated chunk
		return nil
	}

	chunkCollection.add(chunk)

	if chunkCollection.totalSize == chunkCollection.accumulatedSize {
		transportMessageBytes := chunkCollection.assemble()
		assembler.log.Info("assemble chunks successfully", "id", chunk.id, "collection.size", chunkCollection.totalSize)
		transportMessage := &transport.Message{}
		if err := json.Unmarshal(transportMessageBytes, transportMessage); err != nil {
			assembler.log.Error(err, "unmarshal collection bytes to transport.Message error")
			return nil
		}
		return transportMessage
	}
	return nil
}
