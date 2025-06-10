```
kind create cluster --name group-operator
kubectl cluster-info --context kind-group-operator
kind load docker-image coreweave/group-operator:v0.6.0 --name group-operator
kubectl apply --server-side -f deploy/v2beta1/group-operator.yaml --force-conflicts
kubectl apply --server-side -f examples/simple-groupjob.yaml
kubectl get pods -n group-operator-system
```

```
kubectl logs group-operator-5d4bd94888-8kps9  -n group-operator-system
I0610 18:18:08.540016       1 server.go:92] Using cluster scoped operator
I0610 18:18:08.540246       1 server.go:98] [API Version: v2 Version: v0.6.0 Git SHA: f794ae28ee9f24f15184a707f4a90620e90312cc Built: 2025-06-10 06:05:18 Go Version: go1.23.10 Go OS/Arch: linux/arm64]
I0610 18:18:08.540251       1 server.go:101] Server options: &{Kubeconfig: MasterURL: Threadiness:2 MonitoringPort:0 PrintVersion:false GangSchedulingName: Namespace: LockNamespace:group-operator-system QPS:5 Burst:10 ControllerRateLimit:10 ControllerBurst:100}
W0610 18:18:08.540487       1 client_config.go:659] Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.
E0610 18:18:08.544960       1 server.go:316] groupjobs.coreweave.com is forbidden: User "system:serviceaccount:group-operator-system:group-operator" cannot list resource "groupjobs" in API group "coreweave.com" at the cluster scope
I0610 18:18:08.545174       1 leaderelection.go:254] attempting to acquire leader lease group-operator-system/group-operator...
I0610 18:18:08.545205       1 server.go:209] Start listening to 8080 for health check
I0610 18:18:08.546245       1 server.go:258] New leader has been elected: group-operator-5d4bd94888-8kps9_a17fb62c-b86d-4d3b-870c-d78167603725
I0610 18:18:24.190208       1 leaderelection.go:268] successfully acquired lease group-operator-system/group-operator
I0610 18:18:24.190724       1 server.go:247] Leading started
I0610 18:18:24.191587       1 mpi_job_controller.go:349] Setting up informer error handlers
I0610 18:18:24.191633       1 mpi_job_controller.go:376] Setting up event handlers
I0610 18:18:24.191866       1 mpi_job_controller.go:456] Starting GroupJob controller
I0610 18:18:24.191876       1 mpi_job_controller.go:459] Waiting for informer caches to sync
I0610 18:18:24.194168       1 event.go:377] Event(v1.ObjectReference{Kind:"Lease", Namespace:"group-operator-system", Name:"group-operator", UID:"2efbd592-b97f-4e3b-905f-e9f50dbdbfc9", APIVersion:"coordination.k8s.io/v1", ResourceVersion:"56889", FieldPath:""}): type: 'Normal' reason: 'LeaderElection' group-operator-5d4bd94888-8kps9_b599d062-1b0e-4045-a208-493e801a9435 became leader
W0610 18:18:24.205472       1 reflector.go:561] pkg/mod/k8s.io/client-go@v0.31.1/tools/cache/reflector.go:243: failed to list *v2beta1.GroupJob: groupjobs.coreweave.com is forbidden: User "system:serviceaccount:group-operator-system:group-operator" cannot list resource "groupjobs" in API group "coreweave.com" at the cluster scope
E0610 18:18:24.206684       1 reflector.go:158] "Unhandled Error" err="pkg/mod/k8s.io/client-go@v0.31.1/tools/cache/reflector.go:243: Failed to watch *v2beta1.GroupJob: failed to list *v2beta1.GroupJob: groupjobs.coreweave.com is forbidden: User \"system:serviceaccount:group-operator-system:group-operator\" cannot list resource \"groupjobs\" in API group \"coreweave.com\" at the cluster scope" logger="UnhandledError"
F0610 18:18:24.208103       1 mpi_job_controller.go:366] Unable to sync cache for informer mpiJobInformer: failed to list *v2beta1.GroupJob: groupjobs.coreweave.com is forbidden: User "system:serviceaccount:group-operator-system:group-operator" cannot list resource "groupjobs" in API group "coreweave.com" at the cluster scope. Requesting controller to exit.
```
