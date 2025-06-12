```
 2025-06-11T00:13:56.966659441Z I0611 00:13:56.966420       1 server.go:92] Using cluster scoped operator
 2025-06-11T00:13:56.966762958Z I0611 00:13:56.966537       1 server.go:98] [API Version: v2 Version: v0.6.0 Git SHA: 38f97f48d745fd959245bb79e02497b63bb85afd Built: 2025-06-11 00:02:57 Go Version: go1.23.10 Go OS/Arch: linux/amd64]
 2025-06-11T00:13:56.966782289Z I0611 00:13:56.966552       1 server.go:101] Server options: &{Kubeconfig: MasterURL: Threadiness:2 MonitoringPort:0 PrintVersion:false GangSchedulingName: Namespace: LockNamespace:group-operator-system QPS:5 Burst:10 ControllerRateLim
 2025-06-11T00:13:56.966871448Z W0611 00:13:56.966749       1 client_config.go:659] Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.
 2025-06-11T00:13:56.986328271Z I0611 00:13:56.986183       1 leaderelection.go:254] attempting to acquire leader lease group-operator-system/group-operator...
 2025-06-11T00:13:56.987431058Z I0611 00:13:56.986531       1 server.go:209] Start listening to 8080 for health check
 2025-06-11T00:13:56.993320431Z I0611 00:13:56.993073       1 server.go:258] New leader has been elected: group-operator-5d4bd94888-gqths_27d8e3fd-cebd-4e77-a320-ee0c8cfbcbf0
 2025-06-11T00:14:15.055544330Z I0611 00:14:15.054926       1 leaderelection.go:268] successfully acquired lease group-operator-system/group-operator
 2025-06-11T00:14:15.133645144Z I0611 00:14:15.055144       1 server.go:247] Leading started
 2025-06-11T00:14:15.133671765Z I0611 00:14:15.055385       1 event.go:377] Event(v1.ObjectReference{Kind:"Lease", Namespace:"group-operator-system", Name:"group-operator", UID:"2cd19803-6c56-47f4-95fe-9d9460ee5667", APIVersion:"coordination.k8s.io/v1", ResourceVersi
 2025-06-11T00:14:15.133722088Z I0611 00:14:15.055724       1 mpi_job_controller.go:349] Setting up informer error handlers
 2025-06-11T00:14:15.133773768Z I0611 00:14:15.055769       1 mpi_job_controller.go:376] Setting up event handlers
 2025-06-11T00:14:15.133803163Z I0611 00:14:15.055995       1 mpi_job_controller.go:456] Starting GroupJob controller
 2025-06-11T00:14:15.133819574Z I0611 00:14:15.056012       1 mpi_job_controller.go:459] Waiting for informer caches to sync
 2025-06-11T00:14:15.133836342Z E0611 00:14:15.084088       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-wzxrl-launcher\" not found" logger="UnhandledError"
 2025-06-11T00:14:15.156563912Z I0611 00:14:15.156417       1 mpi_job_controller.go:475] Starting workers
 2025-06-11T00:14:15.156604670Z I0611 00:14:15.156455       1 mpi_job_controller.go:481] Started workers
 2025-06-11T00:14:15.156717662Z I0611 00:14:15.156634       1 mpi_job_controller.go:556] Finished syncing job "default/pi" (145.217µs)
 2025-06-11T00:14:15.156789625Z I0611 00:14:15.156659       1 mpi_job_controller.go:538] Successfully synced 'default/pi'
 2025-06-11T00:14:15.245029042Z I0611 00:14:15.244851       1 mpi_job_controller.go:556] Finished syncing job "default/pi" (127.946µs)
 2025-06-11T00:14:15.245078850Z I0611 00:14:15.244887       1 mpi_job_controller.go:538] Successfully synced 'default/pi'
 2025-06-11T00:16:38.209730187Z E0611 00:16:38.209564       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-wzxrl-launcher\" not found" logger="UnhandledError"
 2025-06-11T00:16:41.856625759Z E0611 00:16:41.856483       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-wzxrl-launcher\" not found" logger="UnhandledError"
 2025-06-11T00:16:42.070199471Z E0611 00:16:42.069905       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-wzxrl-launcher\" not found" logger="UnhandledError"
 2025-06-11T00:16:42.076591716Z E0611 00:16:42.076457       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-wzxrl-launcher\" not found" logger="UnhandledError"
 2025-06-11T00:16:45.876387921Z E0611 00:16:45.876187       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-zczpz-launcher\" not found" logger="UnhandledError"
 2025-06-11T00:16:45.885209489Z E0611 00:16:45.884939       1 mpi_job_controller.go:1228] "Unhandled Error" err="obtaining owning k8s Job: job.batch \"hpcv-mpi-zczpz-launcher\" not found" logger="UnhandledError"
```
