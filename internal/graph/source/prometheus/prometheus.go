package prometheus

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/api"
	prom "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type promAPI interface {
	QueryRange(ctx context.Context, query string, r prom.Range, opts ...prom.Option) (model.Value, prom.Warnings, error)
}

type Client struct {
	API    promAPI
	Labels string
}

type roundTripper struct {
	headers map[string]string
}

func NewClient(address string, labels string, headers map[string]string) (*Client, error) {
	c, err := api.NewClient(api.Config{
		Address: address,
		RoundTripper: &roundTripper{
			headers: headers,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	if labels != "" && labels != " " {
		labels = "," + labels
	} else {
		labels = " "
	}

	return &Client{
		API:    prom.NewAPI(c),
		Labels: labels,
	}, nil
}

func (t *roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}

	return http.DefaultTransport.RoundTrip(r)
}
