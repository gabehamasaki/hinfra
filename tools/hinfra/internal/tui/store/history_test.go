package store

import (
	"testing"
	"time"

	"github.com/gabehamasaki/infra/tools/hinfra/internal/actions"
)

func TestSeriesRespeitaCapacidade(t *testing.T) {
	var s Series
	for i := 0; i < historyCapacity+50; i++ {
		s.Push(float64(i))
	}
	if s.Len() != historyCapacity {
		t.Fatalf("Len = %d, esperava %d", s.Len(), historyCapacity)
	}
	if s.Last() != float64(historyCapacity+49) {
		t.Errorf("Last = %v, esperava a amostra mais recente", s.Last())
	}
}

func TestHistoryRecordCalculaTaxaDeRede(t *testing.T) {
	h := NewHistory()
	base := time.Now()

	h.Record(actions.ClusterMetrics{
		FetchedAt: base,
		Nodes: []actions.NodeStat{{
			Name: "srv1", NetRxBytes: 1000, NetTxBytes: 500,
			CPUUsageMilli: 500, CPUCapacityMilli: 2000,
		}},
	})
	// Primeira coleta não tem intervalo anterior, então não gera taxa.
	if h.Node("srv1").NetRx.Len() != 0 {
		t.Fatal("primeira amostra não deveria produzir taxa de rede")
	}

	h.Record(actions.ClusterMetrics{
		FetchedAt: base.Add(10 * time.Second),
		Nodes: []actions.NodeStat{{
			Name: "srv1", NetRxBytes: 3000, NetTxBytes: 1500,
			CPUUsageMilli: 500, CPUCapacityMilli: 2000,
		}},
	})
	if got := h.Node("srv1").NetRx.Last(); got != 200 {
		t.Errorf("taxa rx = %v, esperava 200 B/s ((3000-1000)/10)", got)
	}
	if got := h.Node("srv1").NetTx.Last(); got != 100 {
		t.Errorf("taxa tx = %v, esperava 100 B/s", got)
	}
	if got := h.Node("srv1").CPU.Last(); got != 25 {
		t.Errorf("CPU = %v%%, esperava 25", got)
	}
}

func TestHistoryRecordIgnoraContadorQueZerou(t *testing.T) {
	h := NewHistory()
	base := time.Now()
	h.Record(actions.ClusterMetrics{
		FetchedAt: base,
		Nodes:     []actions.NodeStat{{Name: "srv1", NetRxBytes: 9000}},
	})
	// Kubelet reiniciado: contador cumulativo volta para trás.
	h.Record(actions.ClusterMetrics{
		FetchedAt: base.Add(10 * time.Second),
		Nodes:     []actions.NodeStat{{Name: "srv1", NetRxBytes: 100}},
	})
	if got := h.Node("srv1").NetRx.Last(); got != 0 {
		t.Errorf("taxa = %v, esperava 0 em vez de valor negativo", got)
	}
}
