package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	appctx "github.com/gabehamasaki/infra/tools/mcp/internal/context"
)

type AppParams struct {
	Name          string
	Host          string
	ContainerPort int
	CPURequest    string
	MemoryRequest string
	MemoryLimit   string
	Exposure      string
	ImageRef      string
}

type WorkflowParams struct {
	ImageName string
	AppPath   string
}

type HinfraParams struct {
	App       string
	Image     string
	AppPath   string
	Namespace string
	Host      string
	Exposure  string
}

func RenderAppFiles(p AppParams) (map[string]string, error) {
	defaults(&p)
	files := map[string]string{}
	base := "apps/" + p.Name + "/"
	files[base+"deployment.yaml"], _ = mustRender("deployment", deploymentTemplate, p)
	files[base+"service.yaml"], _ = mustRender("service", serviceTemplate, p)
	files[base+"ingress.yaml"], _ = mustRender("ingress", ingressTemplate, p)
	files[base+"kustomization.yaml"], _ = mustRender("kustomization", kustomizationTemplate, p)
	files["clusters/production/apps/"+p.Name+"-app.yaml"], _ = mustRender("application", applicationTemplate, p)
	return files, nil
}

func defaults(p *AppParams) {
	if p.CPURequest == "" {
		p.CPURequest = "10m"
	}
	if p.MemoryRequest == "" {
		p.MemoryRequest = "32Mi"
	}
	if p.MemoryLimit == "" {
		p.MemoryLimit = "128Mi"
	}
	if p.ContainerPort == 0 {
		p.ContainerPort = 80
	}
	if p.ImageRef == "" {
		p.ImageRef = appctx.ImageRegistry + "/gabehamasaki/" + p.Name
	}
	if p.Exposure == "" {
		p.Exposure = "public"
	}
}

func RenderWorkflow(templateContent string, p WorkflowParams) string {
	out := templateContent
	out = strings.ReplaceAll(out, "gabehamasaki/CHANGE-ME", p.ImageName)
	out = strings.ReplaceAll(out, "apps/CHANGE-ME", p.AppPath)
	return out
}

func RenderHinfra(p HinfraParams) string {
	var b strings.Builder
	b.WriteString("# hinfra.yml — binding explícito projeto ↔ infra\n")
	if p.App != "" {
		b.WriteString("app: " + p.App + "\n")
	}
	if p.Image != "" {
		b.WriteString("image: " + p.Image + "\n")
	}
	if p.AppPath != "" {
		b.WriteString("appPath: " + p.AppPath + "\n")
	}
	if p.Namespace != "" {
		b.WriteString("namespace: " + p.Namespace + "\n")
	}
	if p.Host != "" {
		b.WriteString("host: " + p.Host + "\n")
	}
	if p.Exposure != "" {
		b.WriteString("exposure: " + p.Exposure + "\n")
	}
	return b.String()
}

func WriteFiles(baseDir string, files map[string]string, write bool) (string, error) {
	if !write {
		var b strings.Builder
		for path, content := range files {
			b.WriteString("--- " + path + " ---\n")
			b.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
		return b.String(), nil
	}
	for path, content := range files {
		full := filepath.Join(baseDir, path)
		if _, err := os.Stat(full); err == nil {
			existing, _ := os.ReadFile(full)
			return fmt.Sprintf("arquivo já existe: %s\n--- existente ---\n%s\n--- proposto ---\n%s", full, existing, content), fmt.Errorf("refusing to overwrite %s", full)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	return "arquivos gravados com sucesso", nil
}

func mustRender(name, tmpl string, data interface{}) (string, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func DNSHint(exposure string) string {
	if exposure == "tailnet" {
		return "100.86.241.1"
	}
	return "187.127.62.20"
}

var deploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}
  labels:
    app: {{ .Name }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Name }}
  template:
    metadata:
      labels:
        app: {{ .Name }}
    spec:
      containers:
        - name: {{ .Name }}
          image: {{ .ImageRef }}:latest
          ports:
            - containerPort: {{ .ContainerPort }}
          resources:
            requests:
              cpu: {{ .CPURequest }}
              memory: {{ .MemoryRequest }}
            limits:
              memory: {{ .MemoryLimit }}
          readinessProbe:
            httpGet:
              path: /
              port: {{ .ContainerPort }}
            initialDelaySeconds: 3
          livenessProbe:
            httpGet:
              path: /
              port: {{ .ContainerPort }}
            initialDelaySeconds: 10
`

var serviceTemplate = `apiVersion: v1
kind: Service
metadata:
  name: {{ .Name }}
spec:
  selector:
    app: {{ .Name }}
  ports:
    - port: {{ .ContainerPort }}
      targetPort: {{ .ContainerPort }}
`

var ingressTemplate = `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ .Name }}
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-cloudflare
{{ if eq .Exposure "tailnet" }}    traefik.ingress.kubernetes.io/router.middlewares: "argocd-argocd-tailnet-only@kubernetescrd"
{{ end }}spec:
  ingressClassName: traefik
  tls:
    - hosts:
        - {{ .Host }}
      secretName: {{ .Name }}-tls
  rules:
    - host: {{ .Host }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ .Name }}
                port:
                  number: {{ .ContainerPort }}
`

var kustomizationTemplate = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: {{ .Name }}
resources:
- deployment.yaml
- service.yaml
- ingress.yaml
images:
- name: {{ .ImageRef }}
  newTag: latest
`

var applicationTemplate = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {{ .Name }}
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/gabehamasaki/infra.git
    targetRevision: main
    path: apps/{{ .Name }}
  destination:
    server: https://kubernetes.default.svc
    namespace: {{ .Name }}
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
`
