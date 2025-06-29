# Group Operator

[![CI Status](https://github.com/coreweave/group-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/coreweave/group-operator/actions/workflows/ci.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/coreweave/group-operator)](https://hub.docker.com/r/coreweave/group-operator)


The Group Operator makes it easy to run allreduce-style distributed training on Kubernetes. Please check out [this blog post](https://medium.com/kubeflow/introduction-to-kubeflow-mpi-operator-and-industry-adoption-296d5f2e6edc) for an introduction to Group Operator and its industry adoption.

## Installation

You can deploy the operator with default settings by running the following commands:

- Latest Development Version

```shell
kubectl apply --server-side -f https://raw.githubusercontent.com/coreweave/group-operator/main/deploy/v2beta1/group-operator.yaml
```

- Release Version

```shell
kubectl apply --server-side -f https://raw.githubusercontent.com/coreweave/group-operator/v0.6.0-cw.0/deploy/v2beta1/group-operator.yaml
```


You can check whether the MPI Job custom resource is installed via:

```
kubectl get crd
```

The output should include `groupjobs.coreweave.com` like the following:

```
NAME                                       AGE
...
groupjobs.coreweave.com                       4d
...
```

If it is not included, you can add it as follows using [kustomize](https://github.com/kubernetes-sigs/kustomize):

```bash
git clone https://github.com/coreweave/group-operator
cd group-operator
kustomize build manifests/overlays/kubeflow | kubectl apply -f -
```

Note that since Kubernetes v1.14, `kustomize` became a subcommand in `kubectl` so you can also run the following command instead:

Since Kubernetes v1.21, you can use:

```bash
kubectl apply -k manifests/overlays/kubeflow
```

```bash
kubectl kustomize base | kubectl apply -f -
```

## Creating an MPI Job

You can create an MPI job by defining an `GroupJob` config file. See [TensorFlow benchmark example](examples/v2beta1/tensorflow-benchmarks/tensorflow-benchmarks.yaml) config file for launching a multi-node TensorFlow benchmark training job. You may change the config file based on your requirements.

```
cat examples/v2beta1/tensorflow-benchmarks/tensorflow-benchmarks.yaml
```

Deploy the `GroupJob` resource to start training:

```
kubectl apply -f examples/v2beta1/tensorflow-benchmarks/tensorflow-benchmarks.yaml
```

## Monitoring an MPI Job

Once the `GroupJob` resource is created, you should now be able to see the created pods matching the specified number of GPUs. You can also monitor the job status from the status section. Here is sample output when the job is successfully completed.

```
kubectl get -o yaml groupjobs tensorflow-benchmarks
```

```
apiVersion: kubeflow.org/v2beta1
kind: GroupJob
metadata:
  creationTimestamp: "2019-07-09T22:15:51Z"
  generation: 1
  name: tensorflow-benchmarks
  namespace: default
  resourceVersion: "5645868"
  selfLink: /apis/coreweave.com/v1alpha2/namespaces/default/groupjobs/tensorflow-benchmarks
  uid: 1c5b470f-a297-11e9-964d-88d7f67c6e6d
spec:
  runPolicy:
    cleanPodPolicy: Running
  mpiReplicaSpecs:
    Launcher:
      replicas: 1
      template:
        spec:
          containers:
          - command:
            - mpirun
            - --allow-run-as-root
            - -np
            - "2"
            - -bind-to
            - none
            - -map-by
            - slot
            - -x
            - NCCL_DEBUG=INFO
            - -x
            - LD_LIBRARY_PATH
            - -x
            - PATH
            - -mca
            - pml
            - ob1
            - -mca
            - btl
            - ^openib
            - python
            - scripts/tf_cnn_benchmarks/tf_cnn_benchmarks.py
            - --model=resnet101
            - --batch_size=64
            - --variable_update=horovod
            image: mpioperator/tensorflow-benchmarks:latest
            name: tensorflow-benchmarks
    Worker:
      replicas: 1
      template:
        spec:
          containers:
          - image: mpioperator/tensorflow-benchmarks:latest
            name: tensorflow-benchmarks
            resources:
              limits:
                nvidia.com/gpu: 2
  slotsPerWorker: 2
status:
  completionTime: "2019-07-09T22:17:06Z"
  conditions:
  - lastTransitionTime: "2019-07-09T22:15:51Z"
    lastUpdateTime: "2019-07-09T22:15:51Z"
    message: GroupJob default/tensorflow-benchmarks is created.
    reason: GroupJobCreated
    status: "True"
    type: Created
  - lastTransitionTime: "2019-07-09T22:15:54Z"
    lastUpdateTime: "2019-07-09T22:15:54Z"
    message: GroupJob default/tensorflow-benchmarks is running.
    reason: GroupJobRunning
    status: "False"
    type: Running
  - lastTransitionTime: "2019-07-09T22:17:06Z"
    lastUpdateTime: "2019-07-09T22:17:06Z"
    message: GroupJob default/tensorflow-benchmarks successfully completed.
    reason: GroupJobSucceeded
    status: "True"
    type: Succeeded
  replicaStatuses:
    Launcher:
      succeeded: 1
    Worker: {}
  startTime: "2019-07-09T22:15:51Z"
```

Training should run for 100 steps and takes a few minutes on a GPU cluster. You can inspect the logs to see the training progress. When the job starts, access the logs from the `launcher` pod:

```
PODNAME=$(kubectl get pods -l training.kubeflow.org/job-name=tensorflow-benchmarks,training.kubeflow.org/job-role=launcher -o name)
kubectl logs -f ${PODNAME}
```

```
TensorFlow:  1.14
Model:       resnet101
Dataset:     imagenet (synthetic)
Mode:        training
SingleSess:  False
Batch size:  128 global
             64 per device
Num batches: 100
Num epochs:  0.01
Devices:     ['horovod/gpu:0', 'horovod/gpu:1']
NUMA bind:   False
Data format: NCHW
Optimizer:   sgd
Variables:   horovod

...

40	images/sec: 154.4 +/- 0.7 (jitter = 4.0)	8.280
40	images/sec: 154.4 +/- 0.7 (jitter = 4.1)	8.482
50	images/sec: 154.8 +/- 0.6 (jitter = 4.0)	8.397
50	images/sec: 154.8 +/- 0.6 (jitter = 4.2)	8.450
60	images/sec: 154.5 +/- 0.5 (jitter = 4.1)	8.321
60	images/sec: 154.5 +/- 0.5 (jitter = 4.4)	8.349
70	images/sec: 154.5 +/- 0.5 (jitter = 4.0)	8.433
70	images/sec: 154.5 +/- 0.5 (jitter = 4.4)	8.430
80	images/sec: 154.8 +/- 0.4 (jitter = 3.6)	8.199
80	images/sec: 154.8 +/- 0.4 (jitter = 3.8)	8.404
90	images/sec: 154.6 +/- 0.4 (jitter = 3.7)	8.418
90	images/sec: 154.6 +/- 0.4 (jitter = 3.6)	8.459
100	images/sec: 154.2 +/- 0.4 (jitter = 4.0)	8.372
100	images/sec: 154.2 +/- 0.4 (jitter = 4.0)	8.542
----------------------------------------------------------------
total images/sec: 308.27
```

For a sample that uses Intel MPI, see:

```bash
cat examples/pi/pi-intel.yaml
```

For a sample that uses MPICH, see:

```bash
cat examples/pi/pi-mpich.yaml
```

## Exposed Metrics

| Metric name | Metric type | Description | Labels |
| ----------- | ----------- | ----------- | ------ |
|group\_operator\_jobs\_created\_total | Counter  | Counts number of Group jobs created | |
|group\_operator\_jobs\_successful\_total | Counter  | Counts number of Group jobs successful | |
|group\_operator\_jobs\_failed\_total | Counter  | Counts number of Group jobs failed| |
|group\_operator\_job\_info | Gauge | Information about GroupJob | `launcher`=&lt;launcher-pod-name&gt; <br> `namespace`=&lt;job-namespace&gt; |

### Join Metrics

With [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics), one can join metrics by labels.
For example `kube_pod_info * on(pod,namespace) group_left label_replace(group_operator_job_infos, "pod", "$0", "launcher", ".*")`

## Docker Images

We push Docker images of [coreweave/group-operator Docker image](https://hub.docker.com/r/coreweave/group-operator) for every release.
You can use the following Dockerfile to build the image yourself:

- [group-operator](https://github.com/coreweave/group-operator/blob/master/Dockerfile)

Alternative, you can build the image using make:

```bash
make RELEASE_VERSION=dev IMAGE_NAME=registry.example.com/group-operator images
```

This will produce an image with the tag `registry.example.com/group-operator:dev`.

## Contributing

Learn more in [CONTRIBUTING](https://github.com/coreweave/group-operator/blob/master/CONTRIBUTING.md).
