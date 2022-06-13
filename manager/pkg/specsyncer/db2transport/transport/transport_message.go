package transport

// Message abstracts a message object to be used by different transport components.
type Message struct {
	Destination string `json:"destination"`
	ID          string `json:"id"`
	MsgType     string `json:"msgType"`
	Version     string `json:"version"`
	Payload     []byte `json:"payload"`
}
