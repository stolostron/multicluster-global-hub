package kafkaconsumer

// messageFragment represents one fragment of a kafka message.
type messageFragment struct {
	offset uint32
	bytes  []byte
}
