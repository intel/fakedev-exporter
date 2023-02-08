
Fakedev-exporter
================

"fakedev-exporter" is Go based Prometheus metrics exporter providing
simulated device metrics.  It is intended primarily for simulating
GPUs, but could be configured to simulate also other devices.

Eventual use-cases:
* Driver-less E2E Kubernetes device metrics handling validation
* Simulating per-pod device cgroup metrics and limits
* Device handling scalability testing
* Simulating hard to get device HW

Because Kubernetes does not support per-device resources, only
per-node ones, all device resource handling in Kubernetes assumes
devices (on a given node) to have identical capabilities.  Therefore
all devices simulated for a node by "fakedev-exporter" are identical
(for now) i.e. they will have same device ID and metric limits.

See:
* [Design document](docs/README.md)
* [Deployment instructions](deployments/README.md)
* [JSON example configs](configs/)
