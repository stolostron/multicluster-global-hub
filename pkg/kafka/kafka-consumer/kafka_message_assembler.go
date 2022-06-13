package kafkaconsumer

import (
	"math"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

func newKafkaMessageAssembler() *kafkaMessageAssembler {
	return &kafkaMessageAssembler{
		partitionToFragmentCollectionMap: make(map[int32]map[string]*messageFragmentsCollection),
		partitionLowestOffsetMap:         make(map[int32]kafka.Offset),
	}
}

type kafkaMessageAssembler struct {
	partitionToFragmentCollectionMap map[int32]map[string]*messageFragmentsCollection
	partitionLowestOffsetMap         map[int32]kafka.Offset
}

// processFragmentInfo processes a fragment info and returns kafka message if any got assembled, otherwise,
// nil.
func (assembler *kafkaMessageAssembler) processFragmentInfo(fragInfo *messageFragmentInfo) *kafka.Message {
	partition := fragInfo.kafkaMessage.TopicPartition.Partition

	fragmentCollectionMap, found := assembler.partitionToFragmentCollectionMap[partition]
	if !found {
		fragmentCollectionMap = make(map[string]*messageFragmentsCollection)
		assembler.partitionToFragmentCollectionMap[partition] = fragmentCollectionMap
	}

	fragCollection, found := fragmentCollectionMap[fragInfo.key]
	if found && fragCollection.fragmentationTimestamp.After(fragInfo.fragmentationTimestamp) {
		// fragment timestamp < collection timestamp got an outdated fragment
		return nil
	}

	if !found || fragCollection.fragmentationTimestamp.Before(fragInfo.fragmentationTimestamp) {
		// fragmentCollection not found or is hosting outdated fragments
		fragCollection := newMessageFragmentsCollection(fragInfo.totalSize, fragInfo.fragmentationTimestamp)
		fragmentCollectionMap[fragInfo.key] = fragCollection
		fragCollection.add(fragInfo)
		// update the lowest offset on partition if needed
		assembler.addOffsetToPartition(partition, fragCollection.lowestOffset)

		return nil
	}

	// collection exists and the received fragment package should be added
	fragCollection.add(fragInfo)

	// check if got all and assemble
	if fragCollection.totalMessageSize == fragCollection.accumulatedFragmentsSize {
		return assembler.assembleCollection(fragInfo.key, fragCollection)
	}

	return nil
}

func (assembler *kafkaMessageAssembler) assembleCollection(key string,
	collection *messageFragmentsCollection) *kafka.Message {
	assembledBundle, err := collection.assemble()
	if err != nil {
		return nil // cannot assemble, no need to log
	}

	collection.latestKafkaMessage.Value = assembledBundle // hijack message with the highest offset
	partition := collection.latestKafkaMessage.TopicPartition.Partition

	// delete collection from map
	delete(assembler.partitionToFragmentCollectionMap[partition], key)
	// delete collection map if emptied
	if len(assembler.partitionToFragmentCollectionMap[partition]) == 0 {
		delete(assembler.partitionToFragmentCollectionMap, partition)
	}

	// update message offset: set it to that of the lowest (incomplete) collection's offset on partition, or to this
	// collection's highest offset if it is the lowest on partition
	if lowestOffset := assembler.partitionLowestOffsetMap[partition]; lowestOffset != collection.lowestOffset {
		collection.latestKafkaMessage.TopicPartition.Offset = lowestOffset
	} else {
		// update partition offset cache map
		assembler.deleteOffsetOnPartition(partition, collection.lowestOffset)
	}

	// give back the assembled message to be forwarded by the consumer
	return collection.latestKafkaMessage
}

// fixMessageOffset corrects the offset of the received message so that it does not allow for unsafe committing.
func (assembler *kafkaMessageAssembler) fixMessageOffset(msg *kafka.Message) {
	partition := msg.TopicPartition.Partition

	lowestOffsetOnPartition, found := assembler.partitionLowestOffsetMap[partition]
	if !found {
		// no open fragments sequence on partition
		return
	}
	// fix offset so that it does not allow for unsafe committing after processing.
	msg.TopicPartition.Offset = lowestOffsetOnPartition
}

// addOffsetToPartition sets the partition's lowest offset to the given offset if it is.
func (assembler *kafkaMessageAssembler) addOffsetToPartition(partition int32, offset kafka.Offset) {
	if lowestOffset, found := assembler.partitionLowestOffsetMap[partition]; found && offset >= lowestOffset {
		return // there already is a lower offset mapped on this partition, do nothing for now
	}

	assembler.partitionLowestOffsetMap[partition] = offset
}

// deleteOffsetOnPartition updates the partition -> the lowest offset mapping if the offset received is the lowest
// on the given partition. The new lowest offset is calculated and set if found, otherwise the mapping is deleted.
func (assembler *kafkaMessageAssembler) deleteOffsetOnPartition(partition int32, offset kafka.Offset) {
	if lowestOffset, found := assembler.partitionLowestOffsetMap[partition]; !found || offset != lowestOffset {
		return // not used in mapping
	}

	// update new offset
	if lowestOffset := assembler.findLowestOffsetOnPartition(partition); lowestOffset < 0 {
		// no open sequence left on partition
		delete(assembler.partitionLowestOffsetMap, partition)
	} else {
		assembler.partitionLowestOffsetMap[partition] = lowestOffset
	}
}

// findLowestOffsetOnPartition returns the lowest offset on the given partition, returns -1 if none are found.
func (assembler *kafkaMessageAssembler) findLowestOffsetOnPartition(partition int32) kafka.Offset {
	var lowest kafka.Offset = math.MaxInt64

	if collectionMap, found := assembler.partitionToFragmentCollectionMap[partition]; found {
		for _, collection := range collectionMap {
			if collection.lowestOffset < lowest {
				lowest = collection.lowestOffset
			}
		}
	} else {
		return -1
	}

	return lowest
}
