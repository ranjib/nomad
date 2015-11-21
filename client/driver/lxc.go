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

type LXCDriverConfig struct {
	LXCPath   string `mapstructure:"lxc_path"`
	Name      string `mapstructure:"name"`
	CloneFrom string `mapstructure:"clone_from"`
	Template  string `mapstructure:"template"`
	Distro    string `mapstructure:"distro"`
	Release   string `mapstructure:"release"`
	Arch      string `mapstructure:"arch"`
}

type lxcHandle struct {
	logger *log.Logger
	Name   string
	waitCh chan *cstructs.WaitResult
	doneCh chan struct{}
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
	var driverConfig LXCDriverConfig
	if err := mapstructure.WeakDecode(task.Config, &driverConfig); err != nil {
		return nil, err
	}
	if driverConfig.LXCPath == "" {
		driverConfig.LXCPath = lxc.DefaultConfigPath()
	}
	d.logger.Printf("[DEBUG] Using lxcpath: %s", driverConfig.LXCPath)
	if driverConfig.Name == "" {
		return nil, fmt.Errorf("Missing container name for lxc driver")
	}

	var container *lxc.Container
	if driverConfig.CloneFrom == "" {
		c, err := d.createFromTemplate(driverConfig)
		if err != nil {
			return nil, err
		}
		container = c
	} else {
		c, err := d.createByCloning(driverConfig)
		if err != nil {
			return nil, err
		}
		container = c
	}
	d.logger.Printf("[DEBUG] Using lxc name: %s", driverConfig.Name)
	if err := container.Start(); err != nil {
		d.logger.Printf("[WARN] Failed to start container %s", err)
		return nil, err
	}
	h := &lxcHandle{
		Name:   driverConfig.Name,
		logger: d.logger,
		doneCh: make(chan struct{}),
		waitCh: make(chan *cstructs.WaitResult, 1),
	}
	return h, nil
}

func (d *LXCDriver) createByCloning(config LXCDriverConfig) (*lxc.Container, error) {
	c, err := lxc.NewContainer(config.CloneFrom, config.LXCPath)
	if err != nil {
		d.logger.Printf("[WARN] Failed to initialize container object %s", err)
		return nil, err
	}
	if err := c.Clone(config.Name, lxc.DefaultCloneOptions); err != nil {
		return nil, err
	}
	c1, err1 := lxc.NewContainer(config.Name, config.LXCPath)
	if err1 != nil {
		d.logger.Printf("[WARN] Failed to initialize container object %s", err1)
		return nil, err1
	}
	return c1, nil
}

func (d *LXCDriver) createFromTemplate(config LXCDriverConfig) (*lxc.Container, error) {
	if config.Template == "" {
		return nil, fmt.Errorf("Missing template name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc template: %s", config.Template)
	if config.Distro == "" {
		return nil, fmt.Errorf("Missing distro name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc templare option, distro: %s", config.Distro)
	if config.Release == "" {
		return nil, fmt.Errorf("Missing release name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc templare option, release: %s", config.Release)
	if config.Arch == "" {
		return nil, fmt.Errorf("Missing arch name for lxc driver")
	}
	d.logger.Printf("[DEBUG] Using lxc templare option, arch: %s", config.Arch)
	options := lxc.TemplateOptions{
		Template:             config.Template,
		Distro:               config.Distro,
		Release:              config.Release,
		Arch:                 config.Arch,
		FlushCache:           false,
		DisableGPGValidation: false,
	}
	c, err := lxc.NewContainer(config.Name, config.LXCPath)
	if err != nil {
		d.logger.Printf("[WARN] Failed to initialize container object %s", err)
		return nil, err
	}
	if err := c.Create(options); err != nil {
		d.logger.Printf("[WARN] Failed to create container %s", err)
		return nil, err
	}
	return c, nil
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
		waitCh: make(chan *cstructs.WaitResult, 1),
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *lxcHandle) ID() string {
	return h.Name
}

func (h *lxcHandle) WaitCh() chan *cstructs.WaitResult {
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
	h.logger.Warnf("Update is not supported by lxc driver")
	return nil
}
