package scaffold

import (
	"fmt"
	"strings"

	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
)

// AppendEnvironmentOverlayFiles adds overlay kustomizations and Argo Applications for enabled non-production environments.
func AppendEnvironmentOverlayFiles(files map[string]string, appName string, imageRefs []string, envs map[string]appctx.HinfraEnvironment) {
	for key, env := range envs {
		if key == "production" || !env.Enabled {
			continue
		}
		overlayKey := overlayDirName(key)
		if overlayKey == "" {
			continue
		}
		base := fmt.Sprintf("apps/%s/overlays/%s/", appName, overlayKey)
		files[base+"kustomization.yaml"] = renderOverlayKustomization(env.Namespace, imageRefs)
		if env.Host != "" {
			files[base+"ingress-host-patch.yaml"] = renderIngressHostPatch(appName, env.Host)
		}
		appFile := fmt.Sprintf("clusters/production/apps/%s-%s-app.yaml", appName, overlayKey)
		files[appFile] = renderOverlayApplication(appName, overlayKey, env)
	}
}

func overlayDirName(envKey string) string {
	switch envKey {
	case "dev":
		return "dev"
	case "homolog":
		return "homolog"
	default:
		return ""
	}
}

func renderOverlayKustomization(namespace string, imageRefs []string) string {
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("namespace: " + namespace + "\n")
	b.WriteString("resources:\n- ../../\n")
	if len(imageRefs) > 0 {
		b.WriteString("images:\n")
		for _, ref := range imageRefs {
			b.WriteString("- name: " + ref + "\n  newTag: latest\n")
		}
	}
	return b.String()
}

func renderIngressHostPatch(appName, host string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
spec:
  tls:
    - hosts:
        - %s
      secretName: %s-tls
  rules:
    - host: %s
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: %s
                port:
                  number: 80
`, appName, host, appName, host, appName)
}

func renderOverlayApplication(appName, overlayKey string, env appctx.HinfraEnvironment) string {
	appPath := env.AppPath
	if appPath == "" {
		appPath = fmt.Sprintf("apps/%s/overlays/%s", appName, overlayKey)
	}
	ns := env.Namespace
	if ns == "" {
		ns = appName + "-" + overlayKey
	}
	argocdName := appName + "-" + overlayKey
	return fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/gabehamasaki/infra.git
    targetRevision: main
    path: %s
  destination:
    server: https://kubernetes.default.svc
    namespace: %s
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
`, argocdName, appPath, ns)
}
