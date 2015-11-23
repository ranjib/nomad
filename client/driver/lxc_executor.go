package driver

import (
	"fmt"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/nomad/structs"
	lxc "gopkg.in/lxc/go-lxc.v2"
	"log"
	"strconv"
	"time"
)

type LXCExecutor struct {
	logger    *log.Logger
	container *lxc.Container
}

type LXCExecutorConfig struct {
	LXCPath   string `mapstructure:"lxc_path"`
	Name      string `mapstructure:"name"`
	CloneFrom string `mapstructure:"clone_from"`
	Template  string `mapstructure:"template"`
	Distro    string `mapstructure:"distro"`
	Release   string `mapstructure:"release"`
	Arch      string `mapstructure:"arch"`
}

func (config *LXCExecutorConfig) createFromTemplate() (*lxc.Container, error) {
	if config.Template == "" {
		return nil, fmt.Errorf("Missing template name for lxc driver")
	}
	if config.Distro == "" {
		return nil, fmt.Errorf("Missing distro name for lxc driver")
	}
	if config.Release == "" {
		return nil, fmt.Errorf("Missing release name for lxc driver")
	}
	if config.Arch == "" {
		return nil, fmt.Errorf("Missing arch name for lxc driver")
	}
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
		return nil, err
	}
	if err := c.Create(options); err != nil {
		return nil, err
	}
	return c, nil
}

func (config *LXCExecutorConfig) createByCloning() (*lxc.Container, error) {
	c, err := lxc.NewContainer(config.CloneFrom, config.LXCPath)
	if err != nil {
		return nil, err
	}
	if err := c.Clone(config.Name, lxc.DefaultCloneOptions); err != nil {
		return nil, err
	}
	c1, err1 := lxc.NewContainer(config.Name, config.LXCPath)
	if err1 != nil {
		return nil, err1
	}
	return c1, nil
}

func (config *LXCExecutorConfig) Create() (*lxc.Container, error) {
	if config.LXCPath == "" {
		config.LXCPath = lxc.DefaultConfigPath()
	}
	if config.Name == "" {
		return nil, fmt.Errorf("Missing container name for lxc driver")
	}
	var container *lxc.Container
	if config.CloneFrom == "" {
		c, err := config.createFromTemplate()
		if err != nil {
			return nil, err
		}
		container = c
	} else {
		c, err := config.createByCloning()
		if err != nil {
			return nil, err
		}
		container = c
	}
	return container, nil
}

func (e *LXCExecutor) SetCommand() {
}

func (e *LXCExecutor) Wait() *cstructs.WaitResult {
	for {
		if e.container.Running() {
			time.Sleep(5 * time.Second)
		} else {
			return cstructs.NewWaitResult(0, 0, nil)
		}
	}
	return nil
}

func (e *LXCExecutor) Limit(resources *structs.Resources) error {
	if resources.MemoryMB > 0 {
		limit := strconv.Itoa(resources.MemoryMB) + "M"
		if err := e.container.SetConfigItem("lxc.cgroup.memory.limit_in_bytes", limit); err != nil {
			e.logger.Printf("[ERROR] failed to set memory limit to %s. Error: %v", limit, err)
			return err
		}
	}
	if resources.CPU > 2 {
		limit := strconv.Itoa(resources.CPU)
		if err := e.container.SetConfigItem("lxc.cgroup.cpu.shares", limit); err != nil {
			e.logger.Printf("[ERROR] failed to set cpu limit to %s. Error: %v", limit, err)
			return err
		}
	}
	if resources.IOPS > 0 {
		limit := strconv.Itoa(resources.IOPS)
		if err := e.container.SetConfigItem("lxc.cgroup.blkio.weight", limit); err != nil {
			e.logger.Printf("[ERROR] failed to set iops limit to %s. Error: %v", limit, err)
			return err
		}
	}
	return nil
}

func (e *LXCExecutor) Start() error {
	return e.container.Start()
}

func (e *LXCExecutor) Shutdown() error {
	if e.container.Defined() {
		if e.container.State() == lxc.RUNNING {
			if err := e.container.Stop(); err != nil {
				return err
			}
		}
		return e.container.Destroy()
	}
	return nil
}
