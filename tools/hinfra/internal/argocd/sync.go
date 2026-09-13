package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

type SyncRequest struct {
	Force    bool
	Prune    bool
	Revision string
}

func (c *Client) RequestSync(ctx context.Context, name string, req SyncRequest) error {
	obj, err := c.dynamic.Resource(applicationGVR).Namespace("argocd").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	prune := req.Prune
	if pruneAuto, ok, _ := unstructured.NestedBool(obj.Object, "spec", "syncPolicy", "automated", "prune"); ok {
		prune = pruneAuto
	}

	syncOp := map[string]interface{}{
		"prune": prune,
	}
	if req.Revision != "" {
		syncOp["revision"] = req.Revision
	}
	if req.Force {
		syncOp["syncOptions"] = []string{"Force=true"}
	}

	patch := map[string]interface{}{
		"operation": map[string]interface{}{
			"initiatedBy": map[string]interface{}{"username": "hinfra"},
			"sync":        syncOp,
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = c.dynamic.Resource(applicationGVR).Namespace("argocd").Patch(ctx, name, types.MergePatchType, data, metav1.PatchOptions{})
	return err
}

func (c *Client) WaitForSync(ctx context.Context, name string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			obj, err := c.dynamic.Resource(applicationGVR).Namespace("argocd").Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			phase, _, _ := unstructured.NestedString(obj.Object, "status", "operationState", "phase")
			switch phase {
			case "Succeeded":
				return nil
			case "Failed", "Error":
				msg, _, _ := unstructured.NestedString(obj.Object, "status", "operationState", "message")
				if msg == "" {
					msg = phase
				}
				return fmt.Errorf("sync %s: %s", name, msg)
			}
			syncStatus, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")
			op, found, _ := unstructured.NestedMap(obj.Object, "status", "operationState")
			if (!found || op == nil) && syncStatus == "Synced" {
				return nil
			}
		}
	}
}
