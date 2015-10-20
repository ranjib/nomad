package driver

import (
	"fmt"
	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/nomad/structs"
	lxc "gopkg.in/lxc/go-lxc.v2"
	"log"
)

type LXCDriver struct {
	DriverContext
}

type lxcHandle struct {
	logger *log.Logger
	Name   string
	waitCh chan error
	doneCh chan struct{}
}

func NewLXCDriver(ctx *DriverContext) Driver {
	return &LXCDriver{*ctx}
}

func (d *LXCDriver) Fingerprint(cfg *config.Config, node *structs.Node) (bool, error) {
	node.Attributes["driver.lxc.version"] = lxc.Version()
	node.Attributes["driver.lxc"] = "1"
	d.logger.Printf("[DEBUG] lxc.version: %s", node.Attributes["lxc.version"])
	return true, nil
}

func (d *LXCDriver) Start(ctx *ExecContext, task *structs.Task) (DriverHandle, error) {
	var lxcpath = lxc.DefaultConfigPath()
	path, ok := task.Config["lxcpath"]
	if ok && path != "" {
		lxcpath = path
	}
	d.logger.Printf("[DEBUG] Using lxcpath: %s", lxcpath)
	name, ok := task.Config["name"]
	if !ok || name == "" {
		return nil, fmt.Errorf("Missing container name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc name: %s", name)
	template, ok := task.Config["template"]
	if !ok || template == "" {
		return nil, fmt.Errorf("Missing template name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc template: %s", template)
	distro, ok := task.Config["distro"]
	if !ok || distro == "" {
		return nil, fmt.Errorf("Missing distro name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc templare option, distro: %s", distro)
	release, ok := task.Config["release"]
	if !ok || release == "" {
		return nil, fmt.Errorf("Missing release name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc templare option, release: %s", release)
	arch, ok := task.Config["arch"]
	if !ok || arch == "" {
		return nil, fmt.Errorf("Missing arch name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc templare option, arch: %s", arch)
	options := lxc.TemplateOptions{
		Template:             template,
		Distro:               distro,
		Release:              release,
		Arch:                 arch,
		FlushCache:           false,
		DisableGPGValidation: false,
	}
	c, err := lxc.NewContainer(name, lxcpath)
	if err != nil {
		d.logger.Printf("[WARN] Failed to initialize container object %s", err)
		return nil, err
	}
	if err := c.Create(options); err != nil {
		d.logger.Printf("[WARN] Failed to create container %s", err)
		return nil, err
	}
	if err := c.Start(); err != nil {
		d.logger.Printf("[WARN] Failed to start container %s", err)
		return nil, err
	}
	h := &lxcHandle{
		Name:   name,
		logger: d.logger,
		doneCh: make(chan struct{}),
		waitCh: make(chan error, 1),
	}
	return h, nil
}

func (d *LXCDriver) Open(ctx *ExecContext, name string) (DriverHandle, error) {
	lxcpath := lxc.DefaultConfigPath()
	c, err := lxc.NewContainer(name, lxcpath)
	if err != nil {
		d.logger.Printf("[WARN] Failed to start container %s", err)
		return nil, err
	}
	if err := c.Start(); err != nil {
		d.logger.Printf("[WARN] Failed to start container %s", err)
		return nil, err
	}
	h := &lxcHandle{
		Name:   name,
		logger: d.logger,
		doneCh: make(chan struct{}),
		waitCh: make(chan error, 1),
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *lxcHandle) ID() string {
	return h.Name
}

func (h *lxcHandle) WaitCh() chan error {
	return h.waitCh
}

func (h *lxcHandle) Kill() error {
	lxcpath := lxc.DefaultConfigPath()
	c, err := lxc.NewContainer(h.Name, lxcpath)
	if err != nil {
		h.logger.Printf("[WARN] Failed to initialize container %s", err)
		return err
	}
	if c.Defined() {
		if c.State() == lxc.RUNNING {
			if err := c.Stop(); err != nil {
				h.logger.Printf("[WARN] Failed to stop container %s", err)
				return err
			}
		} else {
			h.logger.Println("[WARN] Container is not running. Skipping stop call")

		}
		if err := c.Destroy(); err != nil {
			h.logger.Printf("[WARN] Failed to destroy container %s", err)
			return err
		}
	} else {
		h.logger.Println("[WARN] Cant kill non-existent container")
	}

	return nil
}

func (h *lxcHandle) Update(task *structs.Task) error {
	return fmt.Errorf("Update is not supported by lxc driver")
}
