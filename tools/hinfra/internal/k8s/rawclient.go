package k8s

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// RawGetter busca caminhos arbitrários da API do Kubernetes. Existe porque
// metrics.k8s.io e a summary API do kubelet não têm cliente tipado no client-go.
type RawGetter interface {
	RawGet(ctx context.Context, path string) ([]byte, error)
}

type restRawGetter struct {
	client rest.Interface
}

func newRawGetter(cfg *rest.Config) (RawGetter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliente sem config REST — métricas exigem conexão real ao cluster")
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &restRawGetter{client: dc.RESTClient()}, nil
}

func (r *restRawGetter) RawGet(ctx context.Context, path string) ([]byte, error) {
	return r.client.Get().AbsPath(path).DoRaw(ctx)
}

// RawGet delega para um RawGetter construído a partir do rest.Config do cliente.
func (c *Client) RawGet(ctx context.Context, path string) ([]byte, error) {
	if c.Raw != nil {
		return c.Raw.RawGet(ctx, path)
	}
	getter, err := newRawGetter(c.Config)
	if err != nil {
		return nil, err
	}
	c.Raw = getter
	return getter.RawGet(ctx, path)
}
