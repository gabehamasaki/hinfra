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

type ApplicationStatus struct {
	Name     string
	Sync     string
	Health   string
	Revision string
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

func RefreshTarget(appName string, sourcePath string, hasChart bool) string {
	if hasChart || strings.HasPrefix(sourcePath, "data-services/") {
		return "root-app"
	}
	if strings.HasPrefix(sourcePath, "apps/") {
		return appName
	}
	return "root-app"
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

func FormatAppStatus(s *ApplicationStatus) string {
	return fmt.Sprintf("sync=%s health=%s revision=%s", s.Sync, s.Health, s.Revision)
}
