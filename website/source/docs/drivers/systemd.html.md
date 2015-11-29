---
layout: "docs"
page_title: "Drivers: systemd"
sidebar_current: "docs-drivers-systemd"
description: |-
  The systemd task driver is used to run executables using transient systemd units.
---

# systemd Driver

Name: `systemd`

The `systemd` driver provides an interface to use [systemd](http://www.freedesktop.org/wiki/Software/systemd/) transient units for running executables with cgroups enforcement. Currently, the driver supports arbitrary executables with resource isolation. It is very similar to the `raw_exec` driver, but allows resource control. systemd task driver uses [dbus api](http://www.freedesktop.org/wiki/Software/systemd/dbus/) and does not support dynamic ports or any sort of chrooting.

## Task Configuration

The `systemd` driver supports the following configuration in the job spec:

* `command` - Executable with all its arguments. Note: executable must have its fully qualified path.
Example:

```
task "sleep" {
  driver = "systemd"
  config {
    command = "/bin/sleep 10"
  }
  resources {
    cpu = 500
    memory = 256
  }
}
```

## Task Directories

The `systemd` driver currently does not support mounting of the `alloc/` and `local/` directory.

## Client Requirements

The `systemd` driver requires systemd to be installed on the agents. Agents need to be run as root.

## Client Attributes

The `systemd` driver will set the following client attributes:

* `driver.systemd` - Set to `1` if systemd is running on the host node.
* `driver.systemd.version` - Version of `systemd` eg: `"225"`

## Resource Isolation

### CPU

Nomad limits containers' CPU based on CPU shares. CPU shares allow containers
to burst past their CPU limits. CPU limits will only be imposed when there is
contention for resources. When the host is under load your process may be
throttled to stabilize QOS depending on how many shares it has. You can see how
many CPU shares are available to your process by reading `NOMAD_CPU_LIMIT`.
1000 shares are approximately equal to 1Ghz.

Please keep the implications of CPU shares in mind when you load test workloads
on Nomad.

### Memory

Nomad limits containers' memory usage based on total virtual memory. This means
that containers scheduled by Nomad cannot use swap. This is to ensure that a
swappy process does not degrade performance for other workloads on the same
host.

Since memory is not an elastic resource, you will need to make sure your
container does not exceed the amount of memory allocated to it, or it will be
terminated or crash when it tries to malloc. A process can inspect its memory
limit by reading `NOMAD_MEMORY_LIMIT`, but will need to track its own memory
usage. Memory limit is expressed in megabytes so 1024 = 1Gb.

### IO

Nomad's uses blkio cgroup, enforced via BlkioWeight systemd directive to throttle filesystem IO.
