package driver

import (
	"fmt"
	"github.com/hashicorp/nomad/client/config"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/client/fingerprint"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	gypsy "github.com/ranjib/gypsy/client"
	lxc "gopkg.in/lxc/go-lxc.v2"
	"log"
	"time"
)

type GypsyDriver struct {
	DriverContext
	fingerprint.StaticFingerprinter
}

type GypsyConfig struct {
	ServerURL string `mapstructure:"server_url"`
	Container string `mapstructure:"container"`
	Pipeline  string `mapstructure:"pipeline"`
	RunId     int    `mapstructure:"run_id"`
}

type gypsyHandle struct {
	logger    *log.Logger
	Id        string
	waitCh    chan *cstructs.WaitResult
	doneCh    chan struct{}
	executor  *LXCExecutor
	Pipeline  string
	RunId     int
	ServerURL string
}

func NewGypsyDriver(ctx *DriverContext) Driver {
	return &GypsyDriver{DriverContext: *ctx}
}

func (d *GypsyDriver) Fingerprint(cfg *config.Config, node *structs.Node) (bool, error) {
	node.Attributes["driver.gypsy.version"] = "0.1"
	node.Attributes["driver.gypsy"] = "1"
	d.logger.Printf("[DEBUG] lxc.version: %s", node.Attributes["driver.gypsy.version"])
	return true, nil
}

func (d *GypsyDriver) Start(ctx *ExecContext, task *structs.Task) (DriverHandle, error) {
	var config GypsyConfig
	if err := mapstructure.WeakDecode(task.Config, &config); err != nil {
		return nil, err
	}
	lxcConfig := &LXCExecutorConfig{
		Name:      ctx.AllocID,
		CloneFrom: config.Container,
	}
	executor, e := NewLXCExecutor(lxcConfig, d.logger)
	d.logger.Printf("[DEBUG] Using lxc name: %s", lxcConfig.Name)
	//envVars := TaskEnvironmentVariables(ctx, task)
	if e != nil {
		d.logger.Printf("[ERROR] failed to create container: %s", e)
		return nil, e
	}
	d.logger.Printf("[DEBUG] Successfully created container: %s", lxcConfig.Name)
	var gypsyServerURL string
	if config.ServerURL == "" {
		gypsyServerURL = "http://127.0.0.1:5678"
	} else {
		gypsyServerURL = config.ServerURL
	}
	h := &gypsyHandle{
		Id:        ctx.AllocID,
		logger:    d.logger,
		doneCh:    make(chan struct{}),
		waitCh:    make(chan *cstructs.WaitResult, 1),
		executor:  executor,
		Pipeline:  config.Pipeline,
		RunId:     config.RunId,
		ServerURL: gypsyServerURL,
	}

	if err := h.executor.Limit(task.Resources); err != nil {
		d.logger.Printf("[WARN] Failed to set resource constraints %s", err)
		return nil, err
	}

	if err := h.executor.Start(); err != nil {
		d.logger.Printf("[WARN] Failed to start container %s", err)
		return nil, err
	}
	go h.run()
	return h, nil
}

func (h *gypsyHandle) performBuild() error {
	container := h.executor.Container()
	h.logger.Printf("[INFO] Waiting for ip allocation of container: ", container.Name())
	container.WaitIPAddresses(30 * time.Second)
	client := gypsy.NewClient(h.ServerURL, h.Pipeline, h.RunId)
	pipeline, err := client.FetchPipeline(h.Pipeline)
	if err != nil {
		h.logger.Printf("[ERR] Failed to fetch pipeline %s", err)
		return err
	}
	err = client.PerformBuild(container, pipeline.Scripts)
	if err != nil {
		h.logger.Printf("[ERR] Failed to build pipeline %s. Error: %v", h.Pipeline, err)
		return err
	}
	if len(pipeline.Artifacts) > 0 {
		err = client.UploadArtifacts(container, pipeline.Artifacts)
		if err != nil {
			h.logger.Printf("[ERR] Failed to upload pipeline %s artifact. Error: %v", h.Pipeline, err)
			return err
		}
	}
	client.Run.Success = true
	client.PostRunData()
	err = client.DestroyContainer(container)
	if err != nil {
		h.logger.Printf("[ERR] Failed to build pipeline %s. Error: %v", h.Pipeline, err)
		return err
	}
	return nil
}
func (h *gypsyHandle) run() {
	var waitResult *cstructs.WaitResult
	err := h.performBuild()
	if err != nil {
		waitResult = cstructs.NewWaitResult(-1, -1, err)
	} else {
		waitResult = cstructs.NewWaitResult(0, 0, nil)
	}
	close(h.doneCh)
	h.waitCh <- waitResult
	close(h.waitCh)
}

func (d *GypsyDriver) Open(ctx *ExecContext, name string) (DriverHandle, error) {
	lxcpath := lxc.DefaultConfigPath()
	c, err := lxc.NewContainer(name, lxcpath)
	if err != nil {
		d.logger.Printf("[WARN] Failed to initialize container %s", err)
		return nil, err
	}
	h := &gypsyHandle{
		Id:     ctx.AllocID,
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

func (h *gypsyHandle) ID() string {
	return fmt.Sprintf("Gypsy:%s", h.Id)
}

func (h *gypsyHandle) WaitCh() chan *cstructs.WaitResult {
	return h.waitCh
}

func (h *gypsyHandle) Kill() error {
	return h.executor.Shutdown()
}

func (h *gypsyHandle) Update(task *structs.Task) error {
	h.logger.Printf("[WARN] Update is not supported by lxc driver")
	return nil
}
