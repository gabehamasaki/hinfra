package store

import (
	"time"

	"github.com/gabehamasaki/hinfra/tools/hinfra/internal/actions"
)

// historyCapacity limita a memória do processo e cobre a largura de qualquer
// terminal razoável — não faz sentido guardar mais amostras do que dá para
// desenhar.
const historyCapacity = 240

// Series é um ring buffer de amostras para alimentar sparkline e área.
type Series struct {
	values []float64
}

func (s *Series) Push(v float64) {
	s.values = append(s.values, v)
	if len(s.values) > historyCapacity {
		s.values = s.values[len(s.values)-historyCapacity:]
	}
}

func (s *Series) Values() []float64 { return s.values }

func (s *Series) Len() int { return len(s.values) }

func (s *Series) Last() float64 {
	if len(s.values) == 0 {
		return 0
	}
	return s.values[len(s.values)-1]
}

// NodeHistory acumula as séries de um node ao longo das coletas.
type NodeHistory struct {
	CPU    Series
	Memory Series
	Disk   Series
	NetRx  Series
	NetTx  Series

	lastRxBytes int64
	lastTxBytes int64
	lastSample  time.Time
}

// History guarda o histórico por node. Vive fora do tea.Model porque o modelo
// é copiado por valor em cada Update, e um ring buffer copiado a cada tecla
// pressionada seria desperdício.
type History struct {
	nodes map[string]*NodeHistory
}

func NewHistory() *History {
	return &History{nodes: map[string]*NodeHistory{}}
}

func (h *History) Node(name string) *NodeHistory {
	if h.nodes == nil {
		h.nodes = map[string]*NodeHistory{}
	}
	if _, ok := h.nodes[name]; !ok {
		h.nodes[name] = &NodeHistory{}
	}
	return h.nodes[name]
}

// Record acrescenta uma amostra por node. Rede vira taxa por segundo, já que os
// contadores do kubelet são cumulativos desde o boot.
func (h *History) Record(metrics actions.ClusterMetrics) {
	for _, node := range metrics.Nodes {
		hist := h.Node(node.Name)
		hist.CPU.Push(node.CPUUsagePercent())
		hist.Memory.Push(node.MemUsagePercent())
		hist.Disk.Push(node.DiskUsagePercent())

		if !hist.lastSample.IsZero() {
			elapsed := metrics.FetchedAt.Sub(hist.lastSample).Seconds()
			if elapsed > 0 {
				hist.NetRx.Push(rate(node.NetRxBytes, hist.lastRxBytes, elapsed))
				hist.NetTx.Push(rate(node.NetTxBytes, hist.lastTxBytes, elapsed))
			}
		}
		hist.lastRxBytes = node.NetRxBytes
		hist.lastTxBytes = node.NetTxBytes
		hist.lastSample = metrics.FetchedAt
	}
}

// rate protege contra reinício do kubelet, que zera os contadores e produziria
// uma taxa negativa absurda.
func rate(current, previous int64, seconds float64) float64 {
	if current < previous {
		return 0
	}
	return float64(current-previous) / seconds
}
