package actions

import (
	"context"
	"sort"
	"time"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/k8s"
)

// NodeStat junta as três fontes de verdade sobre um node: capacity da API core,
// uso instantâneo do metrics-server e disco/rede da summary API do kubelet.
type NodeStat struct {
	Name           string   `json:"name"`
	Ready          bool     `json:"ready"`
	Roles          string   `json:"roles"`
	KubeletVersion string   `json:"kubeletVersion"`
	OSImage        string   `json:"osImage"`
	Conditions     []string `json:"conditions,omitempty"`

	CPUCapacityMilli int64 `json:"cpuCapacityMilli"`
	CPUAllocMilli    int64 `json:"cpuAllocatableMilli"`
	CPUUsageMilli    int64 `json:"cpuUsageMilli"`
	CPURequestsMilli int64 `json:"cpuRequestsMilli"`

	MemCapacityBytes int64 `json:"memCapacityBytes"`
	MemAllocBytes    int64 `json:"memAllocatableBytes"`
	MemUsageBytes    int64 `json:"memUsageBytes"`
	MemRequestsBytes int64 `json:"memRequestsBytes"`

	DiskCapacityBytes int64 `json:"diskCapacityBytes"`
	DiskUsedBytes     int64 `json:"diskUsedBytes"`
	ImageFSUsedBytes  int64 `json:"imageFsUsedBytes"`

	NetRxBytes int64 `json:"netRxBytes"`
	NetTxBytes int64 `json:"netTxBytes"`

	PodCount    int           `json:"podCount"`
	PodCapacity int64         `json:"podCapacity"`
	Uptime      time.Duration `json:"uptime"`

	// SummaryErr registra falha só da summary API: CPU e memória continuam válidos.
	SummaryErr string `json:"summaryErr,omitempty"`
}

func (n NodeStat) CPUUsagePercent() float64 {
	return percent(n.CPUUsageMilli, n.CPUCapacityMilli)
}

func (n NodeStat) CPURequestsPercent() float64 {
	return percent(n.CPURequestsMilli, n.CPUAllocMilli)
}

func (n NodeStat) MemUsagePercent() float64 {
	return percent(n.MemUsageBytes, n.MemCapacityBytes)
}

func (n NodeStat) MemRequestsPercent() float64 {
	return percent(n.MemRequestsBytes, n.MemAllocBytes)
}

func (n NodeStat) DiskUsagePercent() float64 {
	return percent(n.DiskUsedBytes, n.DiskCapacityBytes)
}

func (n NodeStat) PodsPercent() float64 {
	return percent(int64(n.PodCount), n.PodCapacity)
}

func percent(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

type StorageStat struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	CapacityByte int64  `json:"capacityBytes"`
	StorageClass string `json:"storageClass"`
}

type WorkloadStat struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	CPUMilli    int64  `json:"cpuMilli"`
	MemoryBytes int64  `json:"memoryBytes"`
}

type ClusterMetrics struct {
	FetchedAt    time.Time      `json:"fetchedAt"`
	Nodes        []NodeStat     `json:"nodes"`
	TopPods      []WorkloadStat `json:"topPods"`
	PVCs         []StorageStat  `json:"pvcs"`
	TotalPVCByte int64          `json:"totalPvcBytes"`
}

// FetchClusterMetrics agrega tudo numa chamada. Falhas parciais degradam o painel
// em vez de derrubá-lo: sem metrics-server ainda há capacity e requests.
func FetchClusterMetrics(ctx context.Context, env *Env) (ClusterMetrics, error) {
	kc, err := k8s.NewClient(ctx, env.Runtime.Kubeconfig, env.Runtime.TailnetAPI)
	if err != nil {
		return ClusterMetrics{}, err
	}
	nodes, err := k8s.ListNodes(ctx, kc)
	if err != nil {
		return ClusterMetrics{}, err
	}

	usageByNode := map[string]k8s.NodeUsage{}
	if usage, err := k8s.ListNodeUsage(ctx, kc); err == nil {
		for _, u := range usage {
			usageByNode[u.Name] = u
		}
	}
	requestsByNode, _ := k8s.SumPodRequests(ctx, kc)

	stats := make([]NodeStat, 0, len(nodes))
	for _, node := range nodes {
		stat := NodeStat{
			Name:              node.Name,
			Ready:             node.Ready,
			Roles:             node.Roles,
			KubeletVersion:    node.KubeletVersion,
			OSImage:           node.OSImage,
			Conditions:        node.Conditions,
			CPUCapacityMilli:  node.CPUCapacity,
			CPUAllocMilli:     node.CPUAllocatable,
			MemCapacityBytes:  node.MemCapacity,
			MemAllocBytes:     node.MemAllocatable,
			DiskCapacityBytes: node.DiskCapacity,
			PodCapacity:       node.PodCapacity,
		}
		if u, ok := usageByNode[node.Name]; ok {
			stat.CPUUsageMilli = u.CPUMilli
			stat.MemUsageBytes = u.MemoryBytes
		}
		if r, ok := requestsByNode[node.Name]; ok {
			stat.CPURequestsMilli = r.CPUMilli
			stat.MemRequestsBytes = r.MemoryBytes
			stat.PodCount = r.Count
		}
		if summary, err := k8s.GetNodeSummary(ctx, kc, node.Name); err == nil {
			stat.DiskUsedBytes = summary.FSUsedBytes
			if summary.FSCapacityBytes > 0 {
				stat.DiskCapacityBytes = summary.FSCapacityBytes
			}
			stat.ImageFSUsedBytes = summary.ImageFSUsed
			stat.NetRxBytes = summary.NetRxBytes
			stat.NetTxBytes = summary.NetTxBytes
			if !summary.StartTime.IsZero() {
				stat.Uptime = time.Since(summary.StartTime)
			}
			if summary.PodCount > 0 {
				stat.PodCount = summary.PodCount
			}
		} else {
			stat.SummaryErr = err.Error()
		}
		stats = append(stats, stat)
	}

	metrics := ClusterMetrics{FetchedAt: time.Now(), Nodes: stats}

	if pods, err := k8s.ListPodUsage(ctx, kc); err == nil {
		limit := len(pods)
		if limit > 10 {
			limit = 10
		}
		for _, p := range pods[:limit] {
			metrics.TopPods = append(metrics.TopPods, WorkloadStat{
				Namespace: p.Namespace, Name: p.Name,
				CPUMilli: p.CPUMilli, MemoryBytes: p.MemoryBytes,
			})
		}
	}

	if pvcs, err := k8s.ListPVCs(ctx, kc); err == nil {
		for _, pvc := range pvcs {
			metrics.PVCs = append(metrics.PVCs, StorageStat{
				Namespace: pvc.Namespace, Name: pvc.Name, Status: pvc.Status,
				CapacityByte: pvc.CapacityByte, StorageClass: pvc.StorageClass,
			})
			metrics.TotalPVCByte += pvc.CapacityByte
		}
		sort.Slice(metrics.PVCs, func(i, j int) bool {
			return metrics.PVCs[i].CapacityByte > metrics.PVCs[j].CapacityByte
		})
	}

	return metrics, nil
}
