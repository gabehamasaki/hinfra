package k8s

import (
	"context"
	"encoding/json"
	"sort"

	"k8s.io/apimachinery/pkg/api/resource"
)

type nodeMetricsList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Window string `json:"window"`
		Usage  struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"items"`
}

type podMetricsList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Containers []struct {
			Name  string `json:"name"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}

type NodeUsage struct {
	Name        string
	CPUMilli    int64
	MemoryBytes int64
}

type PodUsage struct {
	Namespace   string
	Name        string
	CPUMilli    int64
	MemoryBytes int64
}

func ListNodeUsage(ctx context.Context, c *Client) ([]NodeUsage, error) {
	raw, err := c.RawGet(ctx, "/apis/metrics.k8s.io/v1beta1/nodes")
	if err != nil {
		return nil, err
	}
	return parseNodeUsage(raw)
}

func parseNodeUsage(raw []byte) ([]NodeUsage, error) {
	var list nodeMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]NodeUsage, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, NodeUsage{
			Name:        item.Metadata.Name,
			CPUMilli:    quantityToMilli(item.Usage.CPU),
			MemoryBytes: quantityToBytes(item.Usage.Memory),
		})
	}
	return out, nil
}

// ListPodUsage retorna o consumo por pod, do maior para o menor uso de CPU.
func ListPodUsage(ctx context.Context, c *Client) ([]PodUsage, error) {
	raw, err := c.RawGet(ctx, "/apis/metrics.k8s.io/v1beta1/pods")
	if err != nil {
		return nil, err
	}
	return parsePodUsage(raw)
}

func parsePodUsage(raw []byte) ([]PodUsage, error) {
	var list podMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]PodUsage, 0, len(list.Items))
	for _, item := range list.Items {
		usage := PodUsage{Namespace: item.Metadata.Namespace, Name: item.Metadata.Name}
		for _, container := range item.Containers {
			usage.CPUMilli += quantityToMilli(container.Usage.CPU)
			usage.MemoryBytes += quantityToBytes(container.Usage.Memory)
		}
		out = append(out, usage)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUMilli != out[j].CPUMilli {
			return out[i].CPUMilli > out[j].CPUMilli
		}
		return out[i].MemoryBytes > out[j].MemoryBytes
	})
	return out, nil
}

func quantityToMilli(s string) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.MilliValue()
}

func quantityToBytes(s string) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.Value()
}
