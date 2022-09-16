Using GPU metrics in k8s control plane
======================================

(Not just for monitoring the cluster.)

Introduction
------------

To use the metrics provided to Prometheus for autoscaling and
scheduling decisions, they need to be converted to k8s custom metrics
using Prometheus adapter, as documented e.g. here:
* https://github.com/kubernetes-sigs/prometheus-adapter/blob/master/docs/walkthrough.md
* https://github.com/kubernetes-sigs/prometheus-adapter/blob/master/docs/config-walkthrough.md

For device scheduling decisions, that data needs to be per-device,
which makes custom metrics handling awkward, as each GPU on a node
needs to be addressed separately.

Script and adapter config snippets here offer a little help in doing
that for the faked metrics. They work regardless of whether the
metrics come from `fakedev-exporter`, or from the real device
exporters that it fakes.


Usage
-----

Output from "custom-metrics.sh" can be used as Prometheus adapter configuration:
```
./custom-metrics.sh *-rules.yaml 0/card0/03:00.0 1/card1/0a:00.0 > prometheus-adapter-configMap.yaml
```

After applying the saved output to the cluster:
```
$ kubectl apply -f prometheus-adapter-configMap.yaml
```

Prometheus adapter needs to be redeployed, for it to use the new config:
```
$ kubectl delete -f prometheus-adapter-deployment.yaml
$ kubectl create -f prometheus-adapter-deployment.yaml
```

Checking that the required custom metrics are there, can be done with:
```
$ kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1/ | jq .
```

And for individual metrics with:
```
$ kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1/nodes/*/sysman_power_card0 | jq .
```
