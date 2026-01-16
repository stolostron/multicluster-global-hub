// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"bytes"
	"sort"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// messageChunk represents a chunk of a transport message.
type messageChunk struct {
	id     string
	offset int
	size   int
	bytes  []byte
}

// messageChunksCollection holds a collection of chunks and maintains it until completion.
type messageChunksCollection struct {
	id              string
	totalSize       int
	accumulatedSize int
	chunks          map[int]*messageChunk
	orderedOffsets  []int
	lock            sync.Mutex
}

func newMessageChunksCollection(id string, size int) *messageChunksCollection {
	return &messageChunksCollection{
		id:              id,
		totalSize:       size,
		accumulatedSize: 0,
		chunks:          make(map[int]*messageChunk),
		orderedOffsets:  make([]int, 0),
		lock:            sync.Mutex{},
	}
}

func (collection *messageChunksCollection) add(chunk *messageChunk) {
	collection.lock.Lock()
	defer collection.lock.Unlock()

	// don't add chunk to collection, if already exists.
	if _, found := collection.chunks[chunk.offset]; found {
		return
	}

	collection.chunks[chunk.offset] = chunk
	collection.orderedOffsets = append(collection.orderedOffsets, chunk.offset)
	collection.accumulatedSize += len(chunk.bytes)
}

func (collection *messageChunksCollection) collect() ([]byte, error) {
	collection.lock.Lock()
	defer collection.lock.Unlock()

	sort.Ints(collection.orderedOffsets)
	var buffer bytes.Buffer
	for _, offset := range collection.orderedOffsets {
		_, err := buffer.Write(collection.chunks[offset].bytes)
		if err != nil {
			return nil, err
		}
		collection.chunks[offset].bytes = nil // faster GC
	}
	return buffer.Bytes(), nil
}

type messageAssembler struct {
	log                *zap.SugaredLogger
	lock               sync.Mutex
	chunkCollectionMap map[string]*messageChunksCollection
}

func newMessageAssembler() *messageAssembler {
	return &messageAssembler{
		log:                logger.DefaultZapLogger(),
		lock:               sync.Mutex{},
		chunkCollectionMap: make(map[string]*messageChunksCollection),
	}
}

// processChunk processes a message chunk and returns transport message if any got assembled, otherwise,nil.
func (assembler *messageAssembler) assemble(chunk *messageChunk) []byte {
	assembler.lock.Lock()
	defer assembler.lock.Unlock()

	chunkCollection, found := assembler.chunkCollectionMap[chunk.id]
	if !found {
		chunkCollection = newMessageChunksCollection(chunk.id, chunk.size)
		assembler.chunkCollectionMap[chunk.id] = chunkCollection
	}

	chunkCollection.add(chunk)

	if chunkCollection.totalSize <= chunkCollection.accumulatedSize {
		// delete collection from map
		defer delete(assembler.chunkCollectionMap, chunkCollection.id)

		transportPayloadBytes, err := chunkCollection.collect()
		if err != nil {
			assembler.log.Error(err, "assemble event data failed")
			return nil
		}
		assembler.log.Debugw("assemble event data success!", "id", chunkCollection.id,
			"size", chunkCollection.totalSize)
		return transportPayloadBytes
	}

	return nil
}

func (assembler *messageAssembler) messageChunk(e cloudevents.Event) (*messageChunk, bool) {
	offset, err := types.ToInteger(e.Extensions()[transport.ChunkOffsetKey])
	if err != nil {
		return nil, false
	}

	size, err := types.ToInteger(e.Extensions()[transport.ChunkSizeKey])
	if err != nil {
		return nil, false
	}

	return &messageChunk{
		id:     e.ID(),
		offset: int(offset),
		size:   int(size),
		bytes:  e.Data(),
	}, true
}
