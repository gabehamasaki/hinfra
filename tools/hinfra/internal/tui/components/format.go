package components

import (
	"fmt"
	"time"
)

var byteUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// Bytes formata em unidades binárias com 3 dígitos significativos, para as
// colunas do dashboard ficarem alinhadas sem truncar.
func Bytes(n int64) string {
	if n < 0 {
		return "-"
	}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(byteUnits)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%dB", n)
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f%s", value, byteUnits[unit])
	}
	if value >= 10 {
		return fmt.Sprintf("%.1f%s", value, byteUnits[unit])
	}
	return fmt.Sprintf("%.2f%s", value, byteUnits[unit])
}

// Millicores mostra milicores abaixo de 1 core e cores acima, como o kubectl top.
func Millicores(milli int64) string {
	if milli < 1000 {
		return fmt.Sprintf("%dm", milli)
	}
	return fmt.Sprintf("%.2f", float64(milli)/1000)
}

func Percent(p float64) string {
	return fmt.Sprintf("%.0f%%", p)
}

// Uptime usa a maior unidade relevante — dias e horas para um servidor.
func Uptime(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func Rate(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return "0B/s"
	}
	return Bytes(int64(bytesPerSecond)) + "/s"
}
