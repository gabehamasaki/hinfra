package k8s

import "testing"

const nodeMetricsFixture = `{
 "kind":"NodeMetricsList",
 "items":[{"metadata":{"name":"srv1"},"window":"20s","usage":{"cpu":"195742982n","memory":"3076344Ki"}}]
}`

const podMetricsFixture = `{
 "kind":"PodMetricsList",
 "items":[
  {"metadata":{"name":"low","namespace":"a"},"containers":[{"name":"c","usage":{"cpu":"5m","memory":"10Mi"}}]},
  {"metadata":{"name":"high","namespace":"b"},"containers":[
    {"name":"c1","usage":{"cpu":"100m","memory":"50Mi"}},
    {"name":"c2","usage":{"cpu":"20m","memory":"5Mi"}}]}
 ]
}`

const summaryFixture = `{
 "node":{
  "nodeName":"srv1",
  "startTime":"2026-09-05T05:26:17Z",
  "cpu":{"usageNanoCores":208787169},
  "memory":{"availableBytes":7609188352,"workingSetBytes":717443072},
  "network":{"rxBytes":123,"txBytes":456},
  "fs":{"capacityBytes":102888095744,"usedBytes":7535067136,"availableBytes":95336251392},
  "runtime":{"imageFs":{"usedBytes":3760676864}}
 },
 "pods":[{},{},{}]
}`

func TestParseNodeUsage(t *testing.T) {
	got, err := parseNodeUsage([]byte(nodeMetricsFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("esperava 1 node, veio %d", len(got))
	}
	// 195742982n = 0.195742982 cores; MilliValue arredonda para cima
	if got[0].CPUMilli != 196 {
		t.Errorf("CPUMilli = %d, esperava 196", got[0].CPUMilli)
	}
	if got[0].MemoryBytes != 3076344*1024 {
		t.Errorf("MemoryBytes = %d, esperava %d", got[0].MemoryBytes, 3076344*1024)
	}
}

func TestParsePodUsageSomaContainersEOrdena(t *testing.T) {
	got, err := parsePodUsage([]byte(podMetricsFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 pods, veio %d", len(got))
	}
	if got[0].Name != "high" {
		t.Errorf("primeiro pod = %q, esperava o de maior CPU (high)", got[0].Name)
	}
	if got[0].CPUMilli != 120 {
		t.Errorf("CPUMilli = %d, esperava 120 (100m + 20m)", got[0].CPUMilli)
	}
	if got[0].MemoryBytes != 55*1024*1024 {
		t.Errorf("MemoryBytes = %d, esperava 55Mi", got[0].MemoryBytes)
	}
}

func TestParseNodeSummary(t *testing.T) {
	got, err := parseNodeSummary([]byte(summaryFixture))
	if err != nil {
		t.Fatal(err)
	}
	if got.FSUsedBytes != 7535067136 {
		t.Errorf("FSUsedBytes = %d", got.FSUsedBytes)
	}
	if got.FSCapacityBytes != 102888095744 {
		t.Errorf("FSCapacityBytes = %d", got.FSCapacityBytes)
	}
	if got.NetRxBytes != 123 || got.NetTxBytes != 456 {
		t.Errorf("rede rx=%d tx=%d", got.NetRxBytes, got.NetTxBytes)
	}
	if got.PodCount != 3 {
		t.Errorf("PodCount = %d, esperava 3", got.PodCount)
	}
	if got.StartTime.IsZero() {
		t.Error("StartTime não deveria ser zero")
	}
}

func TestQuantityParsersToleramValorInvalido(t *testing.T) {
	if quantityToMilli("nonsense") != 0 {
		t.Error("esperava 0 para quantidade inválida")
	}
	if quantityToBytes("") != 0 {
		t.Error("esperava 0 para string vazia")
	}
}
