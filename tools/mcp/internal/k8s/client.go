package k8s

import (
	"context"
	"time"

	"github.com/gabehamasaki/infra/tools/mcp/internal/tailnet"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const dialTimeout = 2 * time.Second

type Client struct {
	Clientset kubernetes.Interface
	Config    *rest.Config
}

func NewClient(ctx context.Context, kubeconfig, tailnetAPI string) (*Client, error) {
	if err := tailnet.Require(ctx, tailnetAPI, dialTimeout); err != nil {
		return nil, err
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{Clientset: cs, Config: cfg}, nil
}

func NewClientFromInterface(cs kubernetes.Interface) *Client {
	return &Client{Clientset: cs}
}

type PodSummary struct {
	Name             string
	Phase            string
	Image            string
	Restarts         int32
	WaitingReason    string
	TerminatedReason string
}

func ListPods(ctx context.Context, c *Client, namespace string) ([]PodSummary, error) {
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PodSummary, 0, len(pods.Items))
	for _, p := range pods.Items {
		s := PodSummary{Name: p.Name, Phase: string(p.Status.Phase)}
		if len(p.Spec.Containers) > 0 {
			s.Image = p.Spec.Containers[0].Image
		}
		for _, st := range p.Status.ContainerStatuses {
			s.Restarts += st.RestartCount
			if st.State.Waiting != nil && s.WaitingReason == "" {
				s.WaitingReason = st.State.Waiting.Reason
			}
			if st.State.Terminated != nil && s.TerminatedReason == "" {
				s.TerminatedReason = st.State.Terminated.Reason
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func ListEvents(ctx context.Context, c *Client, namespace string) ([]corev1.Event, error) {
	list, err := c.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetPodLogs(ctx context.Context, c *Client, namespace, pod, container string, previous bool, tail int64) (string, error) {
	opts := &corev1.PodLogOptions{Previous: previous, TailLines: &tail}
	if container != "" {
		opts.Container = container
	}
	req := c.Clientset.CoreV1().Pods(namespace).GetLogs(pod, opts)
	raw, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func RestartDeployment(ctx context.Context, c *Client, namespace, name string) error {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations["infra-mcp/restartedAt"] = time.Now().UTC().Format(time.RFC3339)
	_, err = c.Clientset.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	return err
}
