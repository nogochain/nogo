package websocket

type WSEvent struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type EventSink interface {
	Publish(WSEvent)
}
