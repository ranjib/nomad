package environment

import (
	"fmt"
	"strconv"
	"strings"
)

// A set of environment variables that are exported by each driver.
const (
	// The path to the alloc directory that is shared across tasks within a task
	// group.
	AllocDir = "NOMAD_ALLOC_DIR"

	// The path to the tasks local directory where it can store data that is
	// persisted to the alloc is removed.
	TaskLocalDir = "NOMAD_TASK_DIR"

	// The tasks memory limit in MBs.
	MemLimit = "NOMAD_MEMORY_LIMIT"

	// The tasks limit in MHz.
	CpuLimit = "NOMAD_CPU_LIMIT"

	// The IP address for the task.
	TaskIP = "NOMAD_IP"

	// Prefix for passing both dynamic and static port allocations to
	// tasks.
	// E.g. $NOMAD_PORT_1 or $NOMAD_PORT_http
	PortPrefix = "NOMAD_PORT_"

	// Prefix for passing task meta data.
	MetaPrefix = "NOMAD_META_"
)

var (
	nomadVars = []string{AllocDir, TaskLocalDir, MemLimit, CpuLimit, TaskIP, PortPrefix, MetaPrefix}
)

type TaskEnvironment map[string]string

func NewTaskEnivornment() TaskEnvironment {
	return make(map[string]string)
}

// ParseFromList parses a list of strings with NAME=value pairs and returns a
// TaskEnvironment.
func ParseFromList(envVars []string) (TaskEnvironment, error) {
	t := NewTaskEnivornment()

	for _, pair := range envVars {
		// Start the search from the second byte to skip a possible leading
		// "=". Cmd.exe on Windows creates some special environment variables
		// that start with an "=" and they can be properly retrieved by OS
		// functions so we should handle them properly here.
		idx := strings.Index(pair[1:], "=")
		if idx == -1 {
			return nil, fmt.Errorf("Couldn't parse environment variable: %v", pair)
		}
		idx++ // adjust for slice offset above
		t[pair[:idx]] = pair[idx+1:]
	}

	return t, nil
}

// Returns a list of strings with NAME=value pairs.
func (t TaskEnvironment) List() []string {
	env := []string{}
	for k, v := range t {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

func (t TaskEnvironment) Map() map[string]string {
	return t
}

func (t TaskEnvironment) SetAllocDir(dir string) {
	t[AllocDir] = dir
}

func (t TaskEnvironment) ClearAllocDir() {
	delete(t, AllocDir)
}

func (t TaskEnvironment) SetTaskLocalDir(dir string) {
	t[TaskLocalDir] = dir
}

func (t TaskEnvironment) ClearTaskLocalDir() {
	delete(t, TaskLocalDir)
}

func (t TaskEnvironment) SetMemLimit(limit int) {
	t[MemLimit] = strconv.Itoa(limit)
}

func (t TaskEnvironment) ClearMemLimit() {
	delete(t, MemLimit)
}

func (t TaskEnvironment) SetCpuLimit(limit int) {
	t[CpuLimit] = strconv.Itoa(limit)
}

func (t TaskEnvironment) ClearCpuLimit() {
	delete(t, CpuLimit)
}

func (t TaskEnvironment) SetTaskIp(ip string) {
	t[TaskIP] = ip
}

func (t TaskEnvironment) ClearTaskIp() {
	delete(t, TaskIP)
}

// Takes a map of port labels to their port value.
func (t TaskEnvironment) SetPorts(ports map[string]int) {
	for label, port := range ports {
		t[fmt.Sprintf("%s%s", PortPrefix, label)] = strconv.Itoa(port)
	}
}

func (t TaskEnvironment) ClearPorts() {
	for k, _ := range t {
		if strings.HasPrefix(k, PortPrefix) {
			delete(t, k)
		}
	}
}

// Takes a map of meta values to be passed to the task. The keys are capatilized
// when the environent variable is set.
func (t TaskEnvironment) SetMeta(m map[string]string) {
	for k, v := range m {
		t[fmt.Sprintf("%s%s", MetaPrefix, strings.ToUpper(k))] = v
	}
}

func (t TaskEnvironment) ClearMeta() {
	for k, _ := range t {
		if strings.HasPrefix(k, MetaPrefix) {
			delete(t, k)
		}
	}
}

func (t TaskEnvironment) SetEnvvars(m map[string]string) {
	for k, v := range m {
		t[k] = v
	}
}

func (t TaskEnvironment) ClearEnvvars() {
OUTER:
	for k, _ := range t {
		for _, nomadPrefix := range nomadVars {
			if strings.HasPrefix(k, nomadPrefix) {
				continue OUTER
			}
		}
		delete(t, k)
	}
}
