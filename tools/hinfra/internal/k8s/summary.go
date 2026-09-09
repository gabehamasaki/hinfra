package k8s

import (
	"context"
	"encoding/json"
	"time"
)

// NodeSummary espelha o subconjunto da summary API do kubelet que o dashboard usa.
// É a única fonte de uso real de disco: metrics-server só entrega CPU e memória.
type NodeSummary struct {
	NodeName        string
	StartTime       time.Time
	CPUNanoCores    int64
	MemWorkingSet   int64
	MemAvailable    int64
	FSCapacityBytes int64
	FSUsedBytes     int64
	FSAvailBytes    int64
	ImageFSUsed     int64
	NetRxBytes      int64
	NetTxBytes      int64
	PodCount        int
}

type summaryResponse struct {
	Node struct {
		NodeName  string    `json:"nodeName"`
		StartTime time.Time `json:"startTime"`
		CPU       struct {
			UsageNanoCores int64 `json:"usageNanoCores"`
		} `json:"cpu"`
		Memory struct {
			AvailableBytes  int64 `json:"availableBytes"`
			WorkingSetBytes int64 `json:"workingSetBytes"`
		} `json:"memory"`
		Network struct {
			RxBytes int64 `json:"rxBytes"`
			TxBytes int64 `json:"txBytes"`
		} `json:"network"`
		FS struct {
			CapacityBytes  int64 `json:"capacityBytes"`
			UsedBytes      int64 `json:"usedBytes"`
			AvailableBytes int64 `json:"availableBytes"`
		} `json:"fs"`
		Runtime struct {
			ImageFS struct {
				UsedBytes int64 `json:"usedBytes"`
			} `json:"imageFs"`
		} `json:"runtime"`
	} `json:"node"`
	Pods []json.RawMessage `json:"pods"`
}

func GetNodeSummary(ctx context.Context, c *Client, nodeName string) (NodeSummary, error) {
	raw, err := c.RawGet(ctx, "/api/v1/nodes/"+nodeName+"/proxy/stats/summary")
	if err != nil {
		return NodeSummary{}, err
	}
	return parseNodeSummary(raw)
}

func parseNodeSummary(raw []byte) (NodeSummary, error) {
	var resp summaryResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return NodeSummary{}, err
	}
	n := resp.Node
	return NodeSummary{
		NodeName:        n.NodeName,
		StartTime:       n.StartTime,
		CPUNanoCores:    n.CPU.UsageNanoCores,
		MemWorkingSet:   n.Memory.WorkingSetBytes,
		MemAvailable:    n.Memory.AvailableBytes,
		FSCapacityBytes: n.FS.CapacityBytes,
		FSUsedBytes:     n.FS.UsedBytes,
		FSAvailBytes:    n.FS.AvailableBytes,
		ImageFSUsed:     n.Runtime.ImageFS.UsedBytes,
		NetRxBytes:      n.Network.RxBytes,
		NetTxBytes:      n.Network.TxBytes,
		PodCount:        len(resp.Pods),
	}, nil
}
