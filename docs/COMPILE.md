# Compilation Guide

This project requires Go and `make`.

1. Install Go 1.22 or newer.
2. Run `go mod tidy` to ensure dependencies are present.
3. Build the operator binary:
   ```bash
   make build
   ```
4. Build the container image:
   ```bash
   make images
   ```
5. Run tests:
   ```bash
   make test
   ```

After building the image, deploy the operator:
```bash
kubectl apply -f deploy/v2beta1/group-operator.yaml
```

Use the example job to verify:
```bash
kubectl apply -f examples/simple-groupjob.yaml
```
## Running Locally with kind

1. Create a Kubernetes cluster using [kind](https://kind.sigs.k8s.io/):
   ```bash
   kind create cluster --name group-operator
   ```
2. Load the image into the cluster:
   ```bash
   kind load docker-image registry.example.com/group-operator:dev --name group-operator
   ```
3. Deploy the operator manifest:
   ```bash
   kubectl apply -f deploy/v2beta1/group-operator.yaml
   ```
4. Apply the example job and inspect the pods:
   ```bash
   kubectl apply -f examples/simple-groupjob.yaml
   kubectl get pods -n group-operator-system
   ```
