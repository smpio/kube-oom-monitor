# kube-oom-monitor

`kube-oom-monitor` continuously monitors kernel OOM messages and creates Kubernetes Events with PID and **cgroup**. This information can be used to determine in which container of which pod OOM occurred. It is supposed to be runned as DaemonSet.


## Requiremenets

Container should be run in priveleged context:
  ```
  securityContext:
    privileged: true
  ```


## Usage

```
-nodeName string
    name of the node to bind events (required)
-eventReason string
    event reason (default "NodeOOM")
```


## Background

Kubernetes `node-problem-detector` continuously reads `/dev/kmsg`, [parses](https://github.com/kubernetes/node-problem-detector/blob/7ecb76f31a8b597809835bdc2e0b17dae1cb6f45/config/kernel-monitor.json) OOM messages and creates Kubernetes event `OOMKilling`. But it does not store **cgroup information** of killed process. Without this information it's hard to guess container and pod it relates to.

There is also `SystemOOM` event sometimes [created by](https://github.com/kubernetes/kubernetes/blob/ea0764452222146c47ec826977f49d7001b0ea8c/pkg/kubelet/oom/oom_watcher_linux.go) `kubelet`. But it doesn't pass cgroup too, although it uses [cadvisor oomparser](https://github.com/google/cadvisor/blob/b0c463385753a9734dad8216bf86697b97c11880/utils/oomparser/oomparser.go), that provides it!

`kube-oom-monitor` uses the same cadvisor oomparser.
