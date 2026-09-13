package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// TailnetAPIAddr returns host:port used to verify tailnet reachability before kubectl.
func TailnetAPIAddr(kubeconfigPath, configOverride string) (string, error) {
	if v := strings.TrimSpace(configOverride); v != "" {
		return normalizeHostPort(v)
	}
	if v := strings.TrimSpace(os.Getenv("HINFRA_TAILNET_API")); v != "" {
		return normalizeHostPort(v)
	}
	return apiServerFromKubeconfig(kubeconfigPath)
}

func apiServerFromKubeconfig(path string) (string, error) {
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return "", fmt.Errorf("kubeconfig ilegível em %s: %w", path, err)
	}
	if raw.CurrentContext == "" {
		return "", fmt.Errorf("kubeconfig sem current-context em %s", path)
	}
	ctx, ok := raw.Contexts[raw.CurrentContext]
	if !ok || ctx.Cluster == "" {
		return "", fmt.Errorf("kubeconfig: context %q inválido", raw.CurrentContext)
	}
	cluster, ok := raw.Clusters[ctx.Cluster]
	if !ok || cluster.Server == "" {
		return "", fmt.Errorf("kubeconfig: cluster %q sem server", ctx.Cluster)
	}
	return normalizeHostPort(cluster.Server)
}

func normalizeHostPort(server string) (string, error) {
	s := strings.TrimSpace(server)
	if s == "" || strings.Contains(s, "<") {
		return "", fmt.Errorf("endereço da API inválido: %q", server)
	}
	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", err
		}
		if u.Host == "" {
			return "", fmt.Errorf("URL da API sem host: %q", server)
		}
		return u.Host, nil
	}
	if !strings.Contains(s, ":") {
		return s + ":6443", nil
	}
	return s, nil
}
