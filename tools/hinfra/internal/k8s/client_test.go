package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListPodsAndRestart(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "demo", Image: "ghcr.io/gabehamasaki/demo:abc"}}},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-1", Namespace: "demo"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				RestartCount: 2,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
			}},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "demo", Image: "ghcr.io/gabehamasaki/demo:abc"}}},
	}
	cs := fake.NewSimpleClientset(dep, pod)
	client := NewClientFromInterface(cs)
	ctx := context.Background()
	pods, err := ListPods(ctx, client, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].WaitingReason != "ImagePullBackOff" {
		t.Fatalf("unexpected pods: %+v", pods)
	}
	if err := RestartDeployment(ctx, client, "demo", "demo"); err != nil {
		t.Fatal(err)
	}
}
