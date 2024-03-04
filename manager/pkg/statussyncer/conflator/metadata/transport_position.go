package metadata

type TransportPosition struct {
	Topic     string `json:"-"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	// define the kafka cluster identiy:
	// 1. built in kafka, use the kafka cluster id
	// 2. byo kafka, use the kafka bootstrapserver as the identity
	OwnerIdentity string `json:"ownerIdentity"`
}
