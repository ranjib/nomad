---
layout: "docs"
page_title: "Drivers: LXC"
sidebar_current: "docs-drivers-lxc"
description: |-
  The LXC task driver is used to run system containers using LXC.
---

# LXC Driver

Name: `LXC`

The `LXC` driver provides an interface for using [LXC](https://linuxcontainers.org/) for running
systemd containers. Currently, the driver supports launching
containers and resource isolation, but not dynamic ports. LXC task driver
is capabale of running unprivileged containers (i.e as non-root user).

## Task Configuration

The `LXC` driver supports the following configuration in the job spec:

* `name` - Name of the container that will created
* `clone_from` - If present container will be created by cloning this container
* `template` - Template that will be used to create the container (only used when `clone_from` is absent)
* `distro` - Distro of the container, e.g. Ubuntu (only used when `clone_from` is absent)
* `release` - Release for the distro, e.g. vivid for ubuntu  (only used when `clone_from` is absent)
* `arch` - Arch of the newly created container, e.g. amd64 (only used when `clone_from` is absent)

Example:

```
task "webservice" {
  driver = "lxc"
  config {
    name = "nomad-service-1"
    template = "download"
    distro = "ubuntu"
    release = "vivid"
    arch = "amd64"
  }
  resources {
    cpu = 500
    memory = 256
  }
}
```

## Task Directories

The `LXC` driver currently does not support mounting of the `alloc/` and `local/` directory.

## Client Requirements

The `LXC` driver requires LXC libraries to be installed on the agents.

## Client Attributes

The `LXC` driver will set the following client attributes:

* `driver.lxc` - Set to `1` if LXC is found on the host node.
* `driver.lxc.version` - Version of `LXC` eg: `1.0.8`

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

Nomad's uses blkio cgroup, enforced via lxc.cgroups.blkioweight  LXC config to throttle filesystem IO.

### Security

LXC provides resource isolation by way of [cgroups](https://www.kernel.org/doc/Documentation/cgroups/cgroups.txt) and [namespaces](http://man7.org/linux/man-pages/man7/namespaces.7.html).
