package driver

import (
	"fmt"
	"github.com/hashicorp/nomad/client/config"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/client/fingerprint"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	lxc "gopkg.in/lxc/go-lxc.v2"
	"log"
)

type LXCDriver struct {
	DriverContext
	fingerprint.StaticFingerprinter
}

type lxcHandle struct {
	logger   *log.Logger
	Name     string
	waitCh   chan *cstructs.WaitResult
	doneCh   chan struct{}
	executor *LXCExecutor
}

func NewLXCDriver(ctx *DriverContext) Driver {
	return &LXCDriver{DriverContext: *ctx}
}

func (d *LXCDriver) Fingerprint(cfg *config.Config, node *structs.Node) (bool, error) {
	node.Attributes["driver.lxc.version"] = lxc.Version()
	node.Attributes["driver.lxc"] = "1"
	d.logger.Printf("[DEBUG] lxc.version: %s", node.Attributes["driver.lxc.version"])
	return true, nil
}

func (d *LXCDriver) Start(ctx *ExecContext, task *structs.Task) (DriverHandle, error) {
	var config LXCExecutorConfig
	if err := mapstructure.WeakDecode(task.Config, &config); err != nil {
		return nil, err
	}
	executor := NewLXCExecutor(&config, d.logger)
	if err := executor.Create(); err != nil {
		d.logger.Printf("[ERROR] failed to create container: %s", err)
		return nil, err
	}
	if err := executor.SetupBindMounts(ctx.AllocDir, task.Name); err != nil {
		d.logger.Printf("[ERROR] failed to setup bind mounts: %s", err)
		return nil, err
	}
	//envVars := TaskEnvironmentVariables(ctx, task)
	d.logger.Printf("[DEBUG] Successfully created container: %s", config.Name)
	h := &lxcHandle{
		Name:     config.Name,
		logger:   d.logger,
		doneCh:   make(chan struct{}),
		waitCh:   make(chan *cstructs.WaitResult, 1),
		executor: executor,
	}

	if err := h.executor.Limit(task.Resources); err != nil {
		d.logger.Printf("[WARN] Failed to set resource constraints %s", err)
		return nil, err
	}

	if err := h.executor.Start(); err != nil {
		d.logger.Printf("[ERROR] Failed to start container %s", err)
		return nil, err
	}
	go h.run()
	return h, nil
}

func (h *lxcHandle) run() {
	waitResult := h.executor.Wait()
	close(h.doneCh)
	h.waitCh <- waitResult
	close(h.waitCh)
}

func (d *LXCDriver) Open(ctx *ExecContext, name string) (DriverHandle, error) {
	lxcpath := lxc.DefaultConfigPath()
	c, err := lxc.NewContainer(name, lxcpath)
	if err != nil {
		d.logger.Printf("[WARN] Failed to initialize container %s", err)
		return nil, err
	}
	h := &lxcHandle{
		Name:   name,
		logger: d.logger,
		doneCh: make(chan struct{}),
		waitCh: make(chan *cstructs.WaitResult, 1),
		executor: &LXCExecutor{
			container: c,
			logger:    d.logger,
		},
	}
	return h, nil
}

func (h *lxcHandle) ID() string {
	return fmt.Sprintf("LXC:%s", h.Name)
}

func (h *lxcHandle) WaitCh() chan *cstructs.WaitResult {
	return h.waitCh
}

func (h *lxcHandle) Kill() error {
	return h.executor.Shutdown()
}

func (h *lxcHandle) Update(task *structs.Task) error {
	h.logger.Printf("[WARN] Update is not supported by lxc driver")
	return nil
}
