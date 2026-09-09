package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodeInfoFrom(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "srv1",
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("2"),
				corev1.ResourceMemory:           resource.MustParse("8131476Ki"),
				corev1.ResourceEphemeralStorage: resource.MustParse("100476656Ki"),
				corev1.ResourcePods:             resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8131476Ki"),
			},
			NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.36.4+k3s1", Architecture: "amd64"},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}
	got := nodeInfoFrom(node)
	if !got.Ready {
		t.Error("esperava node Ready")
	}
	if got.CPUCapacity != 2000 {
		t.Errorf("CPUCapacity = %d, esperava 2000 milicores", got.CPUCapacity)
	}
	if got.PodCapacity != 110 {
		t.Errorf("PodCapacity = %d", got.PodCapacity)
	}
	if got.Roles != "control-plane" {
		t.Errorf("Roles = %q", got.Roles)
	}
	if len(got.Conditions) != 1 || got.Conditions[0] != "MemoryPressure" {
		t.Errorf("Conditions = %v, esperava só MemoryPressure ativa", got.Conditions)
	}
}

func TestNodeRolesSemLabelViraWorker(t *testing.T) {
	if got := nodeRoles(map[string]string{"kubernetes.io/os": "linux"}); got != "worker" {
		t.Errorf("nodeRoles = %q, esperava worker", got)
	}
}

func TestSumPodRequestsIgnoraTerminados(t *testing.T) {
	mkPod := func(name, node string, phase corev1.PodPhase, cpu string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: node,
				Containers: []corev1.Container{{
					Name: "c",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: phase},
		}
	}
	cs := fake.NewSimpleClientset(
		&corev1.PodList{Items: []corev1.Pod{
			mkPod("running", "srv1", corev1.PodRunning, "100m"),
			mkPod("done", "srv1", corev1.PodSucceeded, "500m"),
			mkPod("unscheduled", "", corev1.PodPending, "300m"),
		}},
	)
	client := NewClientFromInterface(cs)
	got, err := SumPodRequests(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if got["srv1"].CPUMilli != 100 {
		t.Errorf("CPUMilli = %d, esperava 100 (só o pod running)", got["srv1"].CPUMilli)
	}
	if got["srv1"].Count != 1 {
		t.Errorf("Count = %d, esperava 1", got["srv1"].Count)
	}
}
