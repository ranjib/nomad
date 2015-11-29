package driver

import (
	"fmt"
	systemd "github.com/coreos/go-systemd/dbus"
	"github.com/coreos/go-systemd/util"
	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/client/driver/executor"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/client/fingerprint"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	"log"
)

type SystemdDriverConfig struct {
	Command string `mapstructure:"command"`
}

type SystemdDriver struct {
	DriverContext
	fingerprint.StaticFingerprinter
}

type systemdHandle struct {
	logger   *log.Logger
	Conn     *systemd.Conn
	waitCh   chan *cstructs.WaitResult
	doneCh   chan struct{}
	executor *executor.SystemdExecutor
}

func NewSystemdDriver(ctx *DriverContext) Driver {
	return &SystemdDriver{DriverContext: *ctx}
}

func (d *SystemdDriver) Fingerprint(cfg *config.Config, node *structs.Node) (bool, error) {
	if !util.IsRunningSystemd() {
		return false, nil
	}
	conn, err := systemd.New()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	v, err := conn.GetProperty("Version")
	if err != nil {
		return false, err
	}
	node.Attributes["driver.systemd.version"] = v
	node.Attributes["driver.systemd"] = "1"
	d.logger.Printf("[DEBUG] systemd version : %s\n", v)
	return true, nil
}

func (d *SystemdDriver) Start(ctx *ExecContext, task *structs.Task) (DriverHandle, error) {
	var config SystemdDriverConfig
	if err := mapstructure.WeakDecode(task.Config, &config); err != nil {
		d.logger.Printf("[ERROR] Failed to decode systemd driver config. Error: %s\n", err)
		return nil, err
	}
	h := &systemdHandle{
		logger:   d.logger,
		doneCh:   make(chan struct{}),
		waitCh:   make(chan *cstructs.WaitResult, 1),
		executor: executor.NewSystemdExecutor(ctx.AllocID, config.Command, d.logger),
	}

	if err := h.executor.Limit(task.Resources); err != nil {
		d.logger.Printf("[WARN] Failed to set resource constraints %s", err)
		return nil, err
	}

	if err := h.executor.Start(); err != nil {
		d.logger.Printf("[WARN] Failed to start systemd executor %s", err)
		return nil, err
	}
	go h.run()
	return h, nil
}

func (h *systemdHandle) run() {
	waitResult := h.executor.Wait()
	close(h.doneCh)
	h.waitCh <- waitResult
	close(h.waitCh)
}

func (d *SystemdDriver) Open(ctx *ExecContext, _ string) (DriverHandle, error) {
	h := &systemdHandle{
		logger:   d.logger,
		doneCh:   make(chan struct{}),
		waitCh:   make(chan *cstructs.WaitResult, 1),
		executor: executor.NewSystemdExecutor(ctx.AllocID, "", d.logger),
	}
	return h, nil
}

func (h *systemdHandle) ID() string {
	return fmt.Sprintf("systemd:%s", 1)
}

func (h *systemdHandle) WaitCh() chan *cstructs.WaitResult {
	return h.waitCh
}

func (h *systemdHandle) Kill() error {
	return h.executor.Shutdown()
}

func (h *systemdHandle) Update(task *structs.Task) error {
	h.logger.Printf("[WARN] Update is not supported by lxc driver")
	return nil
}
