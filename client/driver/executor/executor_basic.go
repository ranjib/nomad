// +build !linux

package executor

import (
	cgroupConfig "github.com/opencontainers/runc/libcontainer/configs"
)

func (e *UniversalExecutor) configureChroot() error {
	return nil
}

func DestroyCgroup(groups *cgroupConfig.Cgroup) error {
	return nil
}

func (e *UniversalExecutor) removeChrootMounts() error {
	return nil
}

func (e *UniversalExecutor) runAs(userid string) error {
	return nil
}

func (e *UniversalExecutor) applyLimits(pid int) error {
	return nil
}

func (e *UniversalExecutor) configureIsolation() error {
	return nil
}
