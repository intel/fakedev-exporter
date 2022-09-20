# Deploying and testing fake GPU devices

Contents:
* [Prerequisites](#prerequisites)
  * [GPU plugin](#gpu-plugin)
  * [Other requirements](#other-requirements)
* [Usage](#usage)
  * [Start fakedev-exporter](#start-fakedev-exporter)
  * [Start fakedev workloads](#start-fakedev-workloads)

NOTE: This is work in progress due to missing ready-to-use dependencies.


## Prerequisites

### GPU plugin

While it's possible to use `fakedev-exporter` in k8s cluster without a
device plugin (YAMLs have few comments about that), it's not really
that useful.  Therefore deployment relies on a (faked) GPU plugin
being present.

For now, one needs to:

1. Build related images manually from these WiP pull requests:
   * GPU plugin: https://github.com/intel/intel-device-plugins-for-kubernetes/pull/1114
   * Fake device generator: https://github.com/intel/intel-device-plugins-for-kubernetes/pull/1116

2. Push resulting images to a local registry, and apply following after updating image URLs:
   * Their k8s integration: https://github.com/intel/intel-device-plugins-for-kubernetes/pull/1118

Note that fake GPU plugin provides same 'i915' GPU resource as real
GPU plugin.

If cluster runs both fake and real plugin versions, fake GPU plugin
config should specify a different label for the fake nodes, that can
be used with the `fakedev-exporter` (and its workload) deployment
`nodeSelector`, like the example deployments do.


### Other requirements

GPU plugin uses NFD (node-feature-discovery) for labeling the nodes,
so NFD is also needed.  See GPU plugin installation instructions:
* https://github.com/intel/intel-device-plugins-for-kubernetes/tree/main/cmd/gpu_plugin

Finally, one needs to build `fakedev-exporter` image, push it to some
registry and update its URLs to `fakedev-*.yaml` files.

For metrics reporting to work, _Prometheus_ and `fakedev-exporter`
need to run in the same namespace.  If that's not the case, update
everything shown by `git grep monitoring`.

Workloads run in `validation` namespace.  If fake GPU plugin
deployment did not provide that, add it with:
```
kubectl apply -f workloads/validation-namespace.yaml
```


## Usage

### Start fakedev-exporter

Create roles + services used by `fakedev-exporter`:
```
kubectl apply -f common/
```

Check that `nodeSelector` value and selected `fakedev-exporter` config
content really match [1] platform name and memory amounts provided by
fake GPU plugin config.

Then start `fakedev-exporter` on nodes providing specific fake GPU type:
```
kubectl apply -f ./
```

[1] especially in case of SR-IOV, matching things is trickier because
PF and its VFs typically have different amount of memory and there's
only a subset of metrics available for VFs.


### Start fakedev workloads

Start suitable batch of fake workloads (WLs) with the same
`nodeSelector` as what `fakedev-exporter` uses:
```
kubectl apply -f workloads/fakedev-workload-batch.yaml
```

Each workload instance will then get one of the fake GPU devices
provided by the GPU plugin, and asks `fakedev-exporter` to generate
GPU metrics based on specified fake load on that particular fake GPU,
which are pulled by _Prometheus_ to its metrics database.
