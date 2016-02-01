package driver

import (
	"fmt"
	"github.com/hashicorp/nomad/client/allocdir"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/nomad/structs"
	lxc "gopkg.in/lxc/go-lxc.v2"
	"log"
	"path/filepath"
	"strconv"
	"time"
)

type LXCExecutorConfig struct {
	LXCPath     string            `mapstructure:"lxc_path"`
	Name        string            `mapstructure:"name"`
	CloneFrom   string            `mapstructure:"clone_from"`
	Template    string            `mapstructure:"template"`
	Distro      string            `mapstructure:"distro"`
	Release     string            `mapstructure:"release"`
	Arch        string            `mapstructure:"arch"`
	CgroupItems map[string]string `mapstructure:"cgroup_items"`
	ConfigItems map[string]string `mapstructure:"config_items"`
}

func (config *LXCExecutorConfig) ValidateTemplate() error {
	if config.Template == "" {
		return fmt.Errorf("Missing template name for lxc driver")
	}
	if config.Distro == "" {
		return fmt.Errorf("Missing distro name for lxc driver")
	}
	if config.Release == "" {
		return fmt.Errorf("Missing release name for lxc driver")
	}
	if config.Arch == "" {
		return fmt.Errorf("Missing arch name for lxc driver")
	}
	return nil
}

type LXCExecutor struct {
	logger    *log.Logger
	container *lxc.Container
	config    *LXCExecutorConfig
}

func (e *LXCExecutor) Container() *lxc.Container {
	return e.container
}

func NewLXCExecutor(config *LXCExecutorConfig, logger *log.Logger) *LXCExecutor {
	return &LXCExecutor{
		config: config,
		logger: logger,
	}
}

func (executor *LXCExecutor) createFromTemplate() error {
	if err := executor.config.ValidateTemplate(); err != nil {
		return err
	}
	options := lxc.TemplateOptions{
		Template:             executor.config.Template,
		Distro:               executor.config.Distro,
		Release:              executor.config.Release,
		Arch:                 executor.config.Arch,
		FlushCache:           false,
		DisableGPGValidation: false,
	}
	c, err := lxc.NewContainer(executor.config.Name, executor.config.LXCPath)
	if err != nil {
		return err
	}
	if err := c.Create(options); err != nil {
		return err
	}
	executor.container = c
	return nil
}

func (executor *LXCExecutor) createByCloning() error {
	c, err := lxc.NewContainer(executor.config.CloneFrom, executor.config.LXCPath)
	if err != nil {
		return err
	}
	if err := c.Clone(executor.config.Name, lxc.DefaultCloneOptions); err != nil {
		return err
	}
	c1, err1 := lxc.NewContainer(executor.config.Name, executor.config.LXCPath)
	if err1 != nil {
		return err1
	}
	executor.container = c1
	return nil
}

func (executor *LXCExecutor) Create() error {
	if executor.config.LXCPath == "" {
		executor.config.LXCPath = lxc.DefaultConfigPath()
	}
	if executor.config.Name == "" {
		executor.config.Name = structs.GenerateUUID()
	}
	executor.logger.Printf("[DEBUG] Assigned container name: %s\n", executor.config.Name)
	if executor.config.CloneFrom == "" {
		if err := executor.createFromTemplate(); err != nil {
			return err
		}
	} else {
		if err := executor.createByCloning(); err != nil {
			return err
		}
	}
	for k, v := range executor.config.CgroupItems {
		if err := executor.container.SetCgroupItem(k, v); err != nil {
			return err
		}
	}
	for k, v := range executor.config.ConfigItems {
		if err := executor.container.SetConfigItem(k, v); err != nil {
			return err
		}
	}
	return executor.container.SaveConfigFile(executor.container.ConfigFileName())
}

func (executor *LXCExecutor) SetupBindMounts(alloc *allocdir.AllocDir, task string) error {
	shared := alloc.SharedDir
	local, ok := alloc.TaskDirs[task]
	if !ok {
		return fmt.Errorf("Failed to find task local directory: '%s'", task)
	}
	absLocal, err := filepath.Abs(local)
	if err != nil {
		return err
	}
	absShared, err := filepath.Abs(shared)
	if err != nil {
		return err
	}
	options := "none bind,create=dir,rw 0 0"
	localMountEntry := fmt.Sprintf("%s %s %s", absLocal, allocdir.TaskLocal, options)
	sharedMountEntry := fmt.Sprintf("%s %s %s", absShared, allocdir.SharedAllocName, options)
	if err := executor.container.SetConfigItem("lxc.mount.entry", localMountEntry); err != nil {
		return err
	}
	if err := executor.container.SetConfigItem("lxc.mount.entry", sharedMountEntry); err != nil {
		return err
	}
	return executor.container.SaveConfigFile(executor.container.ConfigFileName())
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
