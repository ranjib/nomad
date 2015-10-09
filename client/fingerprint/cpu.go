package fingerprint

import (
	"fmt"
	"log"

	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/shirou/gopsutil/cpu"
)

// CPUFingerprint is used to fingerprint the CPU
type CPUFingerprint struct {
	logger *log.Logger
}

// NewCPUFingerprint is used to create a CPU fingerprint
func NewCPUFingerprint(logger *log.Logger) Fingerprint {
	f := &CPUFingerprint{logger: logger}
	return f
}

func (f *CPUFingerprint) Fingerprint(cfg *config.Config, node *structs.Node) (bool, error) {
	cpuInfo, err := cpu.GetCPUStat()
	if err != nil {
		f.logger.Println("[WARN] Error reading CPU information:", err)
		return false, err
	}

	var mhz float64
	if cpuInfo.CPUMHz > 0 {
		mhz = cpuInfo.CPUMHz
	} else if cpuInfo.CPUMaxMHz > 0 {
		mhz = cpuInfo.CPUMaxMHz
	} else {
	}
	if mhz > 0 {
		node.Attributes["cpu.frequency"] = fmt.Sprintf("%.6f", mhz)
	}

	if cpuInfo.CPUs > 0 {
		node.Attributes["cpu.numcores"] = fmt.Sprintf("%d", cpuInfo.CPUs)
	}

	if mhz > 0 && cpuInfo.CPUs > 0 {
		node.Attributes["cpu.totalcompute"] = fmt.Sprintf("%.6f", mhz)
		if node.Resources == nil {
			node.Resources = &structs.Resources{}
		}

		node.Resources.CPU = int(mhz)
	}
	if cpuInfo.ModelName != "" {
		node.Attributes["cpu.modelname"] = cpuInfo.ModelName
	}

	return true, nil
}
