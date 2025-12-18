package business

import (
	"context"
	"sync"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
)

// connection represents an active edge device connection.
type connection struct {
	metadata     *Metadata
	dispatchChan chan *chatv1.ConnectResponse
	stream       DeviceStream
	mu           sync.RWMutex
}

func (c *connection) ConsumeDispatch(ctx context.Context) *chatv1.ConnectResponse {
	select {
	case <-ctx.Done():
		return nil
	case data := <-c.dispatchChan:
		return data
	}
}

func (c *connection) Lock() {
	c.mu.Lock()
}

func (c *connection) Unlock() {
	c.mu.Unlock()
}

func NewConnection(stream DeviceStream, metadata *Metadata) Connection {

	return &connection{
		metadata:     metadata,
		stream:       stream,
		dispatchChan: make(chan *chatv1.ConnectResponse, 100),
	}
}

func (c *connection) Metadata() *Metadata {
	return c.metadata
}

func (c *connection) Dispatch(evt *chatv1.ConnectResponse) {
	c.dispatchChan <- evt
}

func (c *connection) Stream() DeviceStream {
	return c.stream
}
