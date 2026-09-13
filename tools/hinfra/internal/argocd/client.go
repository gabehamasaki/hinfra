package argocd

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

type Client struct {
	dynamic dynamic.Interface
}

func NewClient(cfg *rest.Config) (*Client, error) {
	d, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{dynamic: d}, nil
}

type Layer string

const (
	LayerProjects Layer = "projects"
	LayerData     Layer = "data"
	LayerPlatform Layer = "platform"
)

type ApplicationStatus struct {
	Name     string
	Sync     string
	Health   string
	Revision string
}

type ApplicationRow struct {
	Name       string
	Sync       string
	Health     string
	Revision   string
	SourcePath string
	HasChart   bool
	Layer      Layer
	Namespace  string
	Version    string
}

func InferLayer(sourcePath string, hasChart bool) Layer {
	if strings.HasPrefix(sourcePath, "apps/") {
		return LayerProjects
	}
	if strings.HasPrefix(sourcePath, "data-services/") || hasChart {
		return LayerData
	}
	return LayerPlatform
}

func (c *Client) GetApplication(ctx context.Context, name string) (*ApplicationStatus, error) {
	obj, err := c.dynamic.Resource(applicationGVR).Namespace("argocd").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	status := &ApplicationStatus{Name: name}
	if sync, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "status"); ok {
		status.Sync = sync
	}
	if health, ok, _ := unstructured.NestedString(obj.Object, "status", "health", "status"); ok {
		status.Health = health
	}
	if rev, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "revision"); ok {
		status.Revision = rev
	}
	return status, nil
}

func (c *Client) Refresh(ctx context.Context, name string) error {
	obj, err := c.dynamic.Resource(applicationGVR).Namespace("argocd").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["argocd.argoproj.io/refresh"] = "hard"
	obj.SetAnnotations(ann)
	_, err = c.dynamic.Resource(applicationGVR).Namespace("argocd").Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func RefreshTarget(appName string, sourcePath string, hasChart bool, rootApp string) string {
	if rootApp == "" {
		rootApp = "root-app"
	}
	if hasChart || strings.HasPrefix(sourcePath, "data-services/") {
		return rootApp
	}
	if strings.HasPrefix(sourcePath, "apps/") {
		return appName
	}
	return rootApp
}

func (c *Client) SourcePath(ctx context.Context, name string) (string, bool, error) {
	obj, err := c.dynamic.Resource(applicationGVR).Namespace("argocd").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", false, err
	}
	path, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "path")
	chart, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "chart")
	return path, chart != "", nil
}

func (c *Client) AnnotateRefreshHard(ctx context.Context, name string) error {
	return c.Refresh(ctx, name)
}

func (c *Client) ListApplications(ctx context.Context) ([]ApplicationRow, error) {
	list, err := c.dynamic.Resource(applicationGVR).Namespace("argocd").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rows := make([]ApplicationRow, 0, len(list.Items))
	for _, obj := range list.Items {
		name := obj.GetName()
		path, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "path")
		chart, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "chart")
		hasChart := chart != ""
		ns, _, _ := unstructured.NestedString(obj.Object, "spec", "destination", "namespace")
		row := ApplicationRow{
			Name: name, SourcePath: path, HasChart: hasChart,
			Layer: InferLayer(path, hasChart), Namespace: ns,
		}
		if sync, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "status"); ok {
			row.Sync = sync
		}
		if health, ok, _ := unstructured.NestedString(obj.Object, "status", "health", "status"); ok {
			row.Health = health
		}
		if rev, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "revision"); ok {
			row.Revision = rev
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func FormatAppStatus(s *ApplicationStatus) string {
	return fmt.Sprintf("sync=%s health=%s revision=%s", s.Sync, s.Health, s.Revision)
}
