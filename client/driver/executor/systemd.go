package executor

import (
	"fmt"
	systemd "github.com/coreos/go-systemd/dbus"
	"github.com/godbus/dbus"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/nomad/structs"
	"log"
	"strings"
	"time"
)

type SystemdExecutor struct {
	Target     string
	Properties []systemd.Property
	logger     *log.Logger
}

func NewSystemdExecutor(id, command string, logger *log.Logger) *SystemdExecutor {
	props := []systemd.Property{systemd.PropExecStart(strings.Fields(command), false)}
	props = append(props, systemd.Property{Name: "DefaultDependencies", Value: dbus.MakeVariant(false)})
	return &SystemdExecutor{
		Target:     "nomad-" + id + ".service",
		Properties: props,
		logger:     logger,
	}
}

func (e *SystemdExecutor) Start() error {
	conn, err := systemd.New()
	if err != nil {
		e.logger.Printf("[ERROR]Failed to connect to dbus. Error: %s\n", err)
		return err
	}
	defer conn.Close()
	statusCh := make(chan string, 1)
	_, dbusErr := conn.StartTransientUnit(e.Target, "replace", e.Properties, statusCh)
	if dbusErr != nil {
		e.logger.Printf("Failed to start transient unit %s. Error:\n", e.Target, dbusErr)
		return dbusErr
	}
	done := <-statusCh
	if done != "done" {
		e.logger.Printf("[ERROR] Job failed : %s\n", done)
		return fmt.Errorf("Failed to enqueue transiet unit. Status: %s\n", done)
	}
	return nil
}

func (e *SystemdExecutor) Limit(resources *structs.Resources) error {
	if resources.MemoryMB > 0 {
		memoryLimit := systemd.Property{
			Name:  "MemoryLimit",
			Value: dbus.MakeVariant(uint64(resources.MemoryMB * 1024 * 1024)),
		}
		e.Properties = append(e.Properties, memoryLimit)
	}
	if resources.CPU > 2 {
		cpuLimit := systemd.Property{
			Name:  "CPUShares",
			Value: dbus.MakeVariant(uint64(resources.CPU)),
		}
		e.Properties = append(e.Properties, cpuLimit)
	}
	if resources.IOPS > 0 {
		iopsLimit := systemd.Property{
			Name:  "BlkioWeight",
			Value: dbus.MakeVariant(uint64(resources.IOPS)),
		}
		e.Properties = append(e.Properties, iopsLimit)
	}
	return nil
}
func (e *SystemdExecutor) Wait() *cstructs.WaitResult {
	conn, err := systemd.New()
	if err != nil {
		e.logger.Printf("[ERROR]Failed to connect to dbus. Error: %s\n", err)
		return cstructs.NewWaitResult(-1, 0, err)
	}
	defer conn.Close()
	statusCh, errCh := conn.SubscribeUnits(5 * time.Second)
	for {
		select {
		case units := <-statusCh:
			for k, u := range units {
				if k == e.Target {
					e.logger.Printf("[DEBUG]Unit changed: %s\n", k)
					if u == nil {
						return cstructs.NewWaitResult(0, 0, nil)
					} else {
						e.logger.Printf("State changed for triggered unit: %#v\n", u)
					}
				}
			}
		case err := <-errCh:
			return cstructs.NewWaitResult(-1, 0, err)
		}
	}
	return nil
}
func (e *SystemdExecutor) Shutdown() error {
	conn, err := systemd.New()
	if err != nil {
		e.logger.Printf("[ERROR]Failed to connect to dbus. Error: %s\n", err)
		return err
	}
	defer conn.Close()
	reschan := make(chan string)
	_, err1 := conn.StopUnit(e.Target, "replace", reschan)
	if err1 != nil {
		e.logger.Printf("[ERROR]Failed to to make stop unit request. Error: %s\n", err1)
		return err1
	}
	done := <-reschan
	if done != "done" {
		e.logger.Printf("Failed to stip unit %s. Status: %s\n", e.Target, done)
		return fmt.Errorf("Failed to stip unit %s. Status: %s\n", e.Target, done)
	}
	return nil
}
