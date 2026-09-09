package cli

import (
	"fmt"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
	"github.com/gabehamasaki/infra/tools/hinfra/internal/tui/components"
	"github.com/spf13/cobra"
)

func newMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Consumo de CPU, memória, disco e rede dos nodes",
		Long: "Junta capacity da API core, uso do metrics-server e disco/rede da " +
			"summary API do kubelet. É a mesma fonte que alimenta o dashboard da TUI.",
		Run: func(_ *cobra.Command, _ []string) {
			env := loadEnvValidated()
			ctx, cancel := ctx60()
			defer cancel()
			out, err := actions.FetchClusterMetrics(ctx, env)
			if err != nil {
				fatal(err)
			}
			if err := printOrJSON(out, func() { printMetrics(out) }); err != nil {
				fatal(err)
			}
		},
	}
	return cmd
}

func printMetrics(metrics actions.ClusterMetrics) {
	for _, node := range metrics.Nodes {
		state := "Ready"
		if !node.Ready {
			state = "NotReady"
		}
		fmt.Printf("%s  %s  %s  up %s\n", node.Name, state, node.KubeletVersion, components.Uptime(node.Uptime))
		fmt.Printf("  cpu      %5s  %s de %s em uso, %s reservado\n",
			components.Percent(node.CPUUsagePercent()),
			components.Millicores(node.CPUUsageMilli),
			components.Millicores(node.CPUCapacityMilli),
			components.Millicores(node.CPURequestsMilli))
		fmt.Printf("  memória  %5s  %s de %s em uso, %s reservado\n",
			components.Percent(node.MemUsagePercent()),
			components.Bytes(node.MemUsageBytes),
			components.Bytes(node.MemCapacityBytes),
			components.Bytes(node.MemRequestsBytes))
		fmt.Printf("  disco    %5s  %s de %s em uso, %s em imagens\n",
			components.Percent(node.DiskUsagePercent()),
			components.Bytes(node.DiskUsedBytes),
			components.Bytes(node.DiskCapacityBytes),
			components.Bytes(node.ImageFSUsedBytes))
		fmt.Printf("  pods     %5s  %d de %d agendados\n",
			components.Percent(node.PodsPercent()), node.PodCount, node.PodCapacity)
		fmt.Printf("  rede            ↓ %s   ↑ %s acumulado\n",
			components.Bytes(node.NetRxBytes), components.Bytes(node.NetTxBytes))
		if node.SummaryErr != "" {
			fmt.Printf("  aviso    summary API do kubelet indisponível: %s\n", node.SummaryErr)
		}
	}

	if len(metrics.TopPods) > 0 {
		fmt.Println("\ntop pods por cpu:")
		for _, pod := range metrics.TopPods {
			fmt.Printf("  %-46s %6s  %9s\n", pod.Namespace+"/"+pod.Name,
				components.Millicores(pod.CPUMilli), components.Bytes(pod.MemoryBytes))
		}
	}

	if len(metrics.PVCs) > 0 {
		fmt.Printf("\npvcs (%s reservados):\n", components.Bytes(metrics.TotalPVCByte))
		for _, pvc := range metrics.PVCs {
			fmt.Printf("  %-36s %9s  %-12s %s\n", pvc.Namespace+"/"+pvc.Name,
				components.Bytes(pvc.CapacityByte), pvc.StorageClass, pvc.Status)
		}
	}
}
