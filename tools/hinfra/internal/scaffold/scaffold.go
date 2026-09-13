package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
	"gopkg.in/yaml.v3"
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

type MonorepoWorkflowParams struct {
	AppPath       string
	Images        map[string]string
	BuildContexts map[string]appctx.BuildContext
}

const (
	workflowPlaceholderAppPath      = "apps/CHANGE-ME"
	workflowPlaceholderImage        = "gabehamasaki/CHANGE-ME"
	workflowPlaceholderAPI          = "gabehamasaki/CHANGE-ME-api"
	workflowPlaceholderWeb          = "gabehamasaki/CHANGE-ME-web"
	workflowPlaceholderContextAPI   = "BUILD_CONTEXT_API"
	workflowPlaceholderFileAPI      = "BUILD_FILE_API"
	workflowPlaceholderContextWeb   = "BUILD_CONTEXT_WEB"
	workflowPlaceholderFileWeb      = "BUILD_FILE_WEB"
)

func WorkflowTemplateName(monorepo bool) string {
	if monorepo {
		return "deploy-workflow-monorepo-template.yml"
	}
	return "deploy-workflow-template.yml"
}

func RenderWorkflow(templateContent string, p WorkflowParams) string {
	out := templateContent
	out = strings.ReplaceAll(out, workflowPlaceholderImage, p.ImageName)
	out = strings.ReplaceAll(out, workflowPlaceholderAppPath, p.AppPath)
	return out
}

func RenderMonorepoWorkflow(templateContent string, p MonorepoWorkflowParams) string {
	out := strings.ReplaceAll(templateContent, workflowPlaceholderAppPath, p.AppPath)
	if api, ok := p.Images["api"]; ok {
		out = strings.ReplaceAll(out, workflowPlaceholderAPI, api)
	}
	if web, ok := p.Images["web"]; ok {
		out = strings.ReplaceAll(out, workflowPlaceholderWeb, web)
	}
	bc := p.BuildContexts
	if len(bc) == 0 {
		bc = appctx.DefaultBuildContexts()
	}
	if apiBC, ok := bc["api"]; ok {
		out = strings.ReplaceAll(out, workflowPlaceholderContextAPI, apiBC.Context)
		out = strings.ReplaceAll(out, workflowPlaceholderFileAPI, apiBC.File)
	}
	if webBC, ok := bc["web"]; ok {
		out = strings.ReplaceAll(out, workflowPlaceholderContextWeb, webBC.Context)
		out = strings.ReplaceAll(out, workflowPlaceholderFileWeb, webBC.File)
	}
	return out
}

type HinfraParams struct {
	App           string
	Image         string
	Images        map[string]string
	BuildContexts map[string]appctx.BuildContext
	AppPath       string
	Namespace     string
	Host          string
	Exposure      string
	Routing       *appctx.HinfraRouting
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

func RenderHinfra(p HinfraParams) string {
	fields := map[string]interface{}{}
	if p.App != "" {
		fields["app"] = p.App
	}
	if p.AppPath != "" {
		fields["appPath"] = p.AppPath
	}
	if p.Namespace != "" {
		fields["namespace"] = p.Namespace
	}
	if p.Host != "" {
		fields["host"] = p.Host
	}
	if p.Exposure != "" {
		fields["exposure"] = p.Exposure
	}
	if len(p.Images) > 0 {
		fields["images"] = p.Images
		if len(p.BuildContexts) > 0 {
			fields["buildContexts"] = p.BuildContexts
		}
		if p.Routing != nil {
			fields["routing"] = p.Routing
		}
	} else if p.Image != "" {
		fields["image"] = p.Image
	}
	var buf bytes.Buffer
	buf.WriteString("# hinfra.yml — binding explícito projeto ↔ infra\n")
	data, err := yaml.Marshal(fields)
	if err != nil {
		return buf.String()
	}
	buf.Write(data)
	return buf.String()
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
