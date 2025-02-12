package writer

import (
	"context"
	"time"

	"github.com/Jeffail/benthos/v3/internal/http"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/message/batch"
	"github.com/Jeffail/benthos/v3/lib/message/roundtrip"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/types"
	"github.com/Jeffail/benthos/v3/lib/util/http/client"
)

//------------------------------------------------------------------------------

// HTTPClientConfig contains configuration fields for the HTTPClient output
// type.
type HTTPClientConfig struct {
	client.Config     `json:",inline" yaml:",inline"`
	BatchAsMultipart  bool               `json:"batch_as_multipart" yaml:"batch_as_multipart"`
	MaxInFlight       int                `json:"max_in_flight" yaml:"max_in_flight"`
	PropagateResponse bool               `json:"propagate_response" yaml:"propagate_response"`
	Batching          batch.PolicyConfig `json:"batching" yaml:"batching"`
}

// NewHTTPClientConfig creates a new HTTPClientConfig with default values.
func NewHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		Config:            client.NewConfig(),
		BatchAsMultipart:  true, // TODO: V4 Set false by default.
		MaxInFlight:       1,    // TODO: Increase this default?
		PropagateResponse: false,
		Batching:          batch.NewPolicyConfig(),
	}
}

//------------------------------------------------------------------------------

// HTTPClient is an output type that sends messages as HTTP requests to a target
// server endpoint.
type HTTPClient struct {
	client *http.Client

	stats metrics.Type
	log   log.Modular

	conf      HTTPClientConfig
	closeChan chan struct{}
}

// NewHTTPClient creates a new HTTPClient writer type.
func NewHTTPClient(
	conf HTTPClientConfig,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (*HTTPClient, error) {
	h := HTTPClient{
		stats:     stats,
		log:       log,
		conf:      conf,
		closeChan: make(chan struct{}),
	}
	var err error
	if h.client, err = http.NewClient(
		conf.Config,
		http.OptSetLogger(h.log),
		http.OptSetManager(mgr),
		// TODO: V4 Remove this
		http.OptSetStats(metrics.Namespaced(h.stats, "client")),
	); err != nil {
		return nil, err
	}
	return &h, nil
}

//------------------------------------------------------------------------------

// ConnectWithContext does nothing.
func (h *HTTPClient) ConnectWithContext(ctx context.Context) error {
	h.log.Infof("Sending messages via HTTP requests to: %s\n", h.conf.URL)
	return nil
}

// Connect does nothing.
func (h *HTTPClient) Connect() error {
	return h.ConnectWithContext(context.Background())
}

// Write attempts to send a message to an HTTP server, this attempt may include
// retries, and if all retries fail an error is returned.
func (h *HTTPClient) Write(msg types.Message) error {
	return h.WriteWithContext(context.Background(), msg)
}

// WriteWithContext attempts to send a message to an HTTP server, this attempt
// may include retries, and if all retries fail an error is returned.
func (h *HTTPClient) WriteWithContext(ctx context.Context, msg types.Message) error {
	resultMsg, err := h.client.Send(ctx, msg, msg)
	if err == nil && h.conf.PropagateResponse {
		msgCopy := msg.Copy()
		parts := make([]types.Part, resultMsg.Len())
		resultMsg.Iter(func(i int, p types.Part) error {
			if i < msgCopy.Len() {
				parts[i] = msgCopy.Get(i)
			} else {
				parts[i] = msgCopy.Get(0)
			}
			parts[i].Set(p.Get())

			p.Metadata().Iter(func(k, v string) error {
				parts[i].Metadata().Set(k, v)
				return nil
			})

			return nil
		})
		msgCopy.SetAll(parts)
		roundtrip.SetAsResponse(msgCopy)
	}
	return err
}

// CloseAsync shuts down the HTTPClient output and stops processing messages.
func (h *HTTPClient) CloseAsync() {
	close(h.closeChan)
	go h.client.Close(context.Background())
}

// WaitForClose blocks until the HTTPClient output has closed down.
func (h *HTTPClient) WaitForClose(timeout time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------
