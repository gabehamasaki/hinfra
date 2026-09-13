package scaffold

import (
	"fmt"
	"strings"

	appctx "github.com/gabehamasaki/infra/tools/hinfra/internal/context"
)

type MonorepoAppParams struct {
	Name              string
	Host              string
	Exposure          string
	ApiPort           int
	WebPort           int
	ApiImageRef       string
	WebImageRef       string
	SecretName        string
	MigrationCommand  string
	NeedsMigration    bool
	NeedsSealedSecret bool
	CPURequest        string
	MemoryRequest     string
	MemoryLimit       string
}

func RenderMonorepoAppFiles(p MonorepoAppParams) (map[string]string, error) {
	defaultMonorepo(&p)
	base := "apps/" + p.Name + "/"
	files := map[string]string{}
	var err error
	files[base+"api-deployment.yaml"], err = mustRender("apiDeployment", apiDeploymentTemplate, p)
	if err != nil {
		return nil, err
	}
	files[base+"web-deployment.yaml"], err = mustRender("webDeployment", webDeploymentTemplate, p)
	if err != nil {
		return nil, err
	}
	files[base+"api-service.yaml"], err = mustRender("apiService", apiServiceTemplate, p)
	if err != nil {
		return nil, err
	}
	files[base+"web-service.yaml"], err = mustRender("webService", webServiceTemplate, p)
	if err != nil {
		return nil, err
	}
	files[base+"ingress.yaml"], err = mustRender("monorepoIngress", monorepoIngressTemplate, p)
	if err != nil {
		return nil, err
	}
	files[base+"kustomization.yaml"] = renderMonorepoKustomization(p)
	if p.NeedsMigration {
		files[base+"migration-job.yaml"], err = mustRender("migrationJob", migrationJobTemplate, p)
		if err != nil {
			return nil, err
		}
	}
	if p.NeedsSealedSecret {
		files[base+"sealed-secret.yaml.example"] = sealedSecretExample(p)
	}
	files["clusters/production/apps/"+p.Name+"-app.yaml"], err = mustRender("application", applicationTemplate, AppParams{
		Name: p.Name, Host: p.Host, Exposure: p.Exposure, ImageRef: p.WebImageRef,
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func defaultMonorepo(p *MonorepoAppParams) {
	if p.ApiPort == 0 {
		p.ApiPort = 80
	}
	if p.WebPort == 0 {
		p.WebPort = 80
	}
	if p.CPURequest == "" {
		p.CPURequest = "10m"
	}
	if p.MemoryRequest == "" {
		p.MemoryRequest = "32Mi"
	}
	if p.MemoryLimit == "" {
		p.MemoryLimit = "128Mi"
	}
	if p.Exposure == "" {
		p.Exposure = "public"
	}
	if p.ApiImageRef == "" {
		p.ApiImageRef = appctx.ImageRegistry + "/gabehamasaki/" + p.Name + "-api"
	}
	if p.WebImageRef == "" {
		p.WebImageRef = appctx.ImageRegistry + "/gabehamasaki/" + p.Name + "-web"
	}
	if p.SecretName == "" {
		p.SecretName = p.Name + "-env"
	}
	if p.MigrationCommand == "" {
		p.MigrationCommand = "echo replace-with-your-migrate-command"
	}
}

func renderMonorepoKustomization(p MonorepoAppParams) string {
	resources := []string{
		"api-deployment.yaml",
		"api-service.yaml",
		"web-deployment.yaml",
		"web-service.yaml",
		"ingress.yaml",
	}
	if p.NeedsMigration {
		resources = append([]string{"migration-job.yaml"}, resources...)
	}
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("namespace: " + p.Name + "\n")
	b.WriteString("resources:\n")
	for _, r := range resources {
		b.WriteString("- " + r + "\n")
	}
	if p.NeedsSealedSecret {
		b.WriteString("# Após hinfra seal secret, adicione: - sealed-secret.yaml\n")
	}
	b.WriteString("images:\n")
	b.WriteString("- name: " + p.ApiImageRef + "\n")
	b.WriteString("  newTag: latest\n")
	b.WriteString("- name: " + p.WebImageRef + "\n")
	b.WriteString("  newTag: latest\n")
	return b.String()
}

func sealedSecretExample(p MonorepoAppParams) string {
	return fmt.Sprintf(`# Gere o SealedSecret e salve como sealed-secret.yaml (não commite segredo em claro):
#   kubectl create secret generic %s -n %s --from-literal=KEY=value --dry-run=client -o yaml > /tmp/secret.yaml
#   hinfra seal secret -f /tmp/secret.yaml -o apps/%s/sealed-secret.yaml
# Depois adicione sealed-secret.yaml ao kustomization e remova este arquivo .example
`, p.SecretName, p.Name, p.Name)
}

const apiDeploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}-api
  labels:
    app: {{ .Name }}-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Name }}-api
  template:
    metadata:
      labels:
        app: {{ .Name }}-api
    spec:
      containers:
        - name: api
          image: {{ .ApiImageRef }}:latest
          ports:
            - containerPort: {{ .ApiPort }}
{{ if .NeedsSealedSecret }}          envFrom:
            - secretRef:
                name: {{ .SecretName }}
{{ end }}          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              memory: 256Mi
          readinessProbe:
            httpGet:
              path: /
              port: {{ .ApiPort }}
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /
              port: {{ .ApiPort }}
            initialDelaySeconds: 15
            periodSeconds: 20
`

const webDeploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}-web
  labels:
    app: {{ .Name }}-web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Name }}-web
  template:
    metadata:
      labels:
        app: {{ .Name }}-web
    spec:
      containers:
        - name: web
          image: {{ .WebImageRef }}:latest
          ports:
            - containerPort: {{ .WebPort }}
          resources:
            requests:
              cpu: {{ .CPURequest }}
              memory: {{ .MemoryRequest }}
            limits:
              memory: {{ .MemoryLimit }}
          readinessProbe:
            httpGet:
              path: /
              port: {{ .WebPort }}
            initialDelaySeconds: 3
          livenessProbe:
            httpGet:
              path: /
              port: {{ .WebPort }}
            initialDelaySeconds: 10
`

const apiServiceTemplate = `apiVersion: v1
kind: Service
metadata:
  name: api
spec:
  selector:
    app: {{ .Name }}-api
  ports:
    - port: {{ .ApiPort }}
      targetPort: {{ .ApiPort }}
`

const webServiceTemplate = `apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: {{ .Name }}-web
  ports:
    - port: {{ .WebPort }}
      targetPort: {{ .WebPort }}
`

const monorepoIngressTemplate = `apiVersion: networking.k8s.io/v1
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
                name: web
                port:
                  number: {{ .WebPort }}
`

const migrationJobTemplate = `apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Name }}-migrate
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
spec:
  ttlSecondsAfterFinished: 86400
  backoffLimit: 3
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: migrate
          image: {{ .ApiImageRef }}:latest
          command: ["sh", "-c", "{{ .MigrationCommand }}"]
{{ if .NeedsSealedSecret }}          envFrom:
            - secretRef:
                name: {{ .SecretName }}
{{ end }}
`
