package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	appctx "github.com/gabehamasaki/hinfra/tools/hinfra/internal/context"
)

const (
	sealedSecretsControllerName      = "sealed-secrets"
	sealedSecretsControllerNamespace = "kube-system"
)

type SealSecretInput struct {
	AppOverride string
	Namespace   string
	SecretName  string
	InputFile   string
	OutputFile  string
	Execute     bool
}

type SealSecretResult struct {
	Command  string `json:"command,omitempty"`
	Output   string `json:"output,omitempty"`
	Message  string `json:"message,omitempty"`
	WroteTo  string `json:"wroteTo,omitempty"`
}

func SealSecret(env *Env, in SealSecretInput) (SealSecretResult, error) {
	app, err := ResolveApp(env, in.AppOverride)
	if err != nil {
		return SealSecretResult{}, err
	}
	ns := in.Namespace
	if ns == "" {
		ns = app.Namespace
	}
	outputPath := in.OutputFile
	if outputPath == "" {
		outputPath = filepath.Join(app.InfraRepo, app.AppPath, "sealed-secret.yaml")
	} else if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(app.InfraRepo, outputPath)
	}
	inputPath := in.InputFile
	if inputPath == "" {
		return SealSecretResult{}, fmt.Errorf("inputFile é obrigatório (Secret kubernetes em yaml)")
	}
	if !filepath.IsAbs(inputPath) {
		inputPath = filepath.Join(app.ProjectRepo, inputPath)
	}
	args := []string{
		"--controller-name=" + sealedSecretsControllerName,
		"--controller-namespace=" + sealedSecretsControllerNamespace,
		"--namespace", ns,
		"--format", "yaml",
	}
	cmdStr := fmt.Sprintf("kubeseal %s < %s > %s",
		strings.Join(args, " "), inputPath, outputPath)
	if !in.Execute {
		return SealSecretResult{
			Command: cmdStr,
			Message: "preview — passe execute:true ou rode o comando localmente com KUBECONFIG na tailnet",
		}, nil
	}
	if _, err := exec.LookPath("kubeseal"); err != nil {
		return SealSecretResult{Command: cmdStr}, fmt.Errorf("kubeseal não encontrado no PATH")
	}
	kubeconfig := app.Kubeconfig
	if kubeconfig == "" {
		kubeconfig = env.Runtime.Kubeconfig
	}
	outFile, err := os.Create(outputPath)
	if err != nil {
		return SealSecretResult{}, err
	}
	defer outFile.Close()
	inFile, err := os.Open(inputPath)
	if err != nil {
		return SealSecretResult{}, err
	}
	defer inFile.Close()
	cmd := exec.Command("kubeseal", args...)
	cmd.Stdin = inFile
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	if err := cmd.Run(); err != nil {
		return SealSecretResult{Command: cmdStr}, err
	}
	return SealSecretResult{
		Command: cmdStr,
		WroteTo: outputPath,
		Message: "sealed-secret gravado em " + outputPath,
	}, nil
}

func DefaultKubesealArgs(namespace string) []string {
	return []string{
		"--controller-name=" + sealedSecretsControllerName,
		"--controller-namespace=" + sealedSecretsControllerNamespace,
		"--namespace", namespace,
		"--format", "yaml",
	}
}

func SealHint(app *appctx.AppContext) string {
	return fmt.Sprintf("kubeseal %s < secret.yaml > %s",
		strings.Join(DefaultKubesealArgs(app.Namespace), " "),
		filepath.Join(app.AppPath, "sealed-secret.yaml"))
}
