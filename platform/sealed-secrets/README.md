# Sealed Secrets - backup da master key

O role `sealed_secrets` (rodado pelo `ansible/site.yml`) instala o controller e imprime,
ao final, o nome do Secret que guarda a master key (label
`sealedsecrets.bitnami.com/sealed-secrets-key=active`, namespace `kube-system`).

**Faça o backup manualmente logo depois da instalação** — sem isso, um cluster novo ou
recriado não decifra os `SealedSecret` já commitados no repo (isso quebra o requisito de
"repo replicável").

```bash
export KUBECONFIG=ansible/.secrets/vps-1.kubeconfig
kubectl get secret -n kube-system -l sealedsecrets.bitnami.com/sealed-secrets-key=active -o yaml > backup-sealed-secrets-key.yaml
```

Guarde `backup-sealed-secrets-key.yaml` fora do cluster e fora do git em texto puro — num
gerenciador de senhas, ou criptografado com `ansible-vault encrypt` num local separado.
**Nunca commite esse arquivo sem criptografia.**

## Restaurar num cluster novo

```bash
kubectl apply -f backup-sealed-secrets-key.yaml
kubectl delete pod -n kube-system -l app.kubernetes.io/name=sealed-secrets
```

## Uso (segredos de projetos, via GitOps)

```bash
kubeseal --format yaml < meu-secret.yaml > apps/<projeto>/sealed-secret.yaml
```

Commite o `sealed-secret.yaml` gerado — é seguro, só o controller do cluster decifra.
