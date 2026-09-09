package k8s

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeInfo struct {
	Name           string
	Ready          bool
	Roles          string
	KubeletVersion string
	OSImage        string
	Architecture   string
	CPUCapacity    int64 // milicores
	CPUAllocatable int64
	MemCapacity    int64 // bytes
	MemAllocatable int64
	DiskCapacity   int64 // ephemeral-storage em bytes
	PodCapacity    int64
	Conditions     []string
}

func ListNodes(ctx context.Context, c *Client) ([]NodeInfo, error) {
	list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]NodeInfo, 0, len(list.Items))
	for _, node := range list.Items {
		out = append(out, nodeInfoFrom(node))
	}
	return out, nil
}

func nodeInfoFrom(node corev1.Node) NodeInfo {
	info := NodeInfo{
		Name:           node.Name,
		KubeletVersion: node.Status.NodeInfo.KubeletVersion,
		OSImage:        node.Status.NodeInfo.OSImage,
		Architecture:   node.Status.NodeInfo.Architecture,
		Roles:          nodeRoles(node.Labels),
	}
	if cpu := node.Status.Capacity.Cpu(); cpu != nil {
		info.CPUCapacity = cpu.MilliValue()
	}
	if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
		info.CPUAllocatable = cpu.MilliValue()
	}
	if mem := node.Status.Capacity.Memory(); mem != nil {
		info.MemCapacity = mem.Value()
	}
	if mem := node.Status.Allocatable.Memory(); mem != nil {
		info.MemAllocatable = mem.Value()
	}
	if disk := node.Status.Capacity.StorageEphemeral(); disk != nil {
		info.DiskCapacity = disk.Value()
	}
	if pods := node.Status.Capacity.Pods(); pods != nil {
		info.PodCapacity = pods.Value()
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			info.Ready = cond.Status == corev1.ConditionTrue
			continue
		}
		// Pressure conditions só interessam quando ativas.
		if cond.Status == corev1.ConditionTrue {
			info.Conditions = append(info.Conditions, string(cond.Type))
		}
	}
	return info
}

func nodeRoles(labels map[string]string) string {
	var roles []string
	for key := range labels {
		if role := strings.TrimPrefix(key, "node-role.kubernetes.io/"); role != key && role != "" {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return "worker"
	}
	return strings.Join(roles, ",")
}

// PodRequests soma os requests declarados de todos os pods agendados no node.
// É o número que importa nesta infra: o orçamento de CPU é o limite real, não o uso.
type PodRequests struct {
	CPUMilli    int64
	MemoryBytes int64
	Count       int
}

func SumPodRequests(ctx context.Context, c *Client) (map[string]PodRequests, error) {
	pods, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := map[string]PodRequests{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		agg := out[pod.Spec.NodeName]
		agg.Count++
		for _, container := range pod.Spec.Containers {
			if cpu := container.Resources.Requests.Cpu(); cpu != nil {
				agg.CPUMilli += cpu.MilliValue()
			}
			if mem := container.Resources.Requests.Memory(); mem != nil {
				agg.MemoryBytes += mem.Value()
			}
		}
		out[pod.Spec.NodeName] = agg
	}
	return out, nil
}

type PVCInfo struct {
	Namespace    string
	Name         string
	Status       string
	CapacityByte int64
	StorageClass string
}

func ListPVCs(ctx context.Context, c *Client) ([]PVCInfo, error) {
	list, err := c.Clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PVCInfo, 0, len(list.Items))
	for _, pvc := range list.Items {
		info := PVCInfo{
			Namespace: pvc.Namespace,
			Name:      pvc.Name,
			Status:    string(pvc.Status.Phase),
		}
		if pvc.Spec.StorageClassName != nil {
			info.StorageClass = *pvc.Spec.StorageClassName
		}
		if cap, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			info.CapacityByte = cap.Value()
		}
		out = append(out, info)
	}
	return out, nil
}
