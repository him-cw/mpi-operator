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
kubectl apply -f deploy/v2beta1/mpi-operator.yaml
```
Use the example job to verify:
```bash
kubectl apply -f examples/simple-groupjob.yaml
```