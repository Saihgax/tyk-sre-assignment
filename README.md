# Tyk SRE Assignment

This repository contains a single Go service for checking Kubernetes workload health, creating workload isolation policies, and reporting Kubernetes API connectivity.

## Requirements

- Go 1.19 or newer
- Docker
- Helm 3
- kubectl
- A Kubernetes cluster such as Minikube

The network-isolation demonstration requires a CNI that enforces Kubernetes NetworkPolicy, such as Calico.

## Project Structure

```text
golang/                       Go application and tests
charts/tyk-sre-assignment/    Helm chart
examples/test-workloads.yaml  api and worker demo workloads
.github/workflows/ci.yml      GitHub Actions workflow
Dockerfile                    multi-stage container build
```

## Run Tests and Build Locally

From the repository root:

```bash
cd golang
go test ./...
go build -o tyk-sre-assignment .
```

Run the application against a Kubernetes cluster:

```bash
./tyk-sre-assignment \
  --kubeconfig "$HOME/.kube/config" \
  --address ":8080"
```

When running inside Kubernetes, leave the kubeconfig flag empty so client-go uses in-cluster configuration.

## HTTP API

### Process health

```bash
curl http://127.0.0.1:8080/healthz
```

Returns `200` when the Go process is serving requests.

### Kubernetes readiness

```bash
curl http://127.0.0.1:8080/readyz
```

Returns `200` and the Kubernetes version when the API server is reachable. Returns `503` when it is not.

### Deployment health

```bash
curl http://127.0.0.1:8080/deployments/health
```

Lists Deployments across namespaces and compares desired replicas with available replicas.

Example response:

```json
{
  "healthy": true,
  "deployments": [
    {
      "namespace": "default",
      "name": "tyk-sre-app",
      "desiredReplicas": 1,
      "availableReplicas": 1,
      "healthy": true
    }
  ]
}
```

### Network isolation

```bash
curl -X POST http://127.0.0.1:8080/network/isolate \
  -H "Content-Type: application/json" \
  -d '{
    "name": "block-api-worker",
    "sourceNamespace": "default",
    "sourceSelector": {"app": "api"},
    "targetNamespace": "default",
    "targetSelector": {"app": "worker"}
  }'
```

The service creates one default-deny ingress and egress NetworkPolicy for each selected workload. This guarantees isolation but also blocks each selected workload from other traffic. Kubernetes NetworkPolicy is allow-list based, so this broader behavior is a documented tradeoff.

## Docker Image

Build the image from the repository root:

```bash
docker build -t tyk-sre-assignment:local .
```

The Dockerfile uses a Go builder stage and a minimal non-root runtime stage. The final image contains only the compiled application binary.

## GitHub Actions

The workflow runs on pull requests and pushes to `main`:

```text
Go tests
Go binary build
Docker image build
```

It validates the image but does not push it to a registry because no registry or credentials are configured.

## Helm Chart

Validate and render the chart locally:

```bash
helm lint charts/tyk-sre-assignment
helm template tyk-sre charts/tyk-sre-assignment
```

Install or upgrade it in the current Kubernetes context:

```bash
helm upgrade --install tyk-sre ./charts/tyk-sre-assignment \
  --set image.repository=tyk-sre-assignment \
  --set image.tag=local \
  --set image.pullPolicy=IfNotPresent
```

The chart creates a Deployment, ClusterIP Service, ServiceAccount, ClusterRole, and ClusterRoleBinding. RBAC allows the application to list Deployments across namespaces and create NetworkPolicies.

## Minikube Demonstration

For Deployment, readiness, and HTTP testing:

```bash
minikube start --driver=docker
docker build -t tyk-sre-assignment:local .
minikube image load tyk-sre-assignment:local
helm upgrade --install tyk-sre ./charts/tyk-sre-assignment \
  --set image.repository=tyk-sre-assignment \
  --set image.tag=local \
  --set image.pullPolicy=IfNotPresent
kubectl get pods
```

For actual NetworkPolicy enforcement, create a fresh cluster with a supported CNI:

```bash
minikube delete
minikube start --driver=docker --cni=calico
docker build -t tyk-sre-assignment:local .
minikube image load tyk-sre-assignment:local
helm upgrade --install tyk-sre ./charts/tyk-sre-assignment \
  --set image.repository=tyk-sre-assignment \
  --set image.tag=local \
  --set image.pullPolicy=IfNotPresent
```

After every new local Docker build, load the updated image into Minikube again:

```bash
minikube image load tyk-sre-assignment:local
```

Access the application from the host:

```bash
kubectl port-forward service/tyk-sre-service 8080:8080
```

In another terminal:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/deployments/health
```

Create the demo workloads:

```bash
kubectl apply -f examples/test-workloads.yaml
kubectl rollout status deployment/api
kubectl rollout status deployment/worker
```

Verify traffic before isolation:

```bash
kubectl exec deployment/api -- wget -qO- -T 3 http://worker.default.svc.cluster.local:8080
```

Expected output:

```text
worker
```

Create and inspect the isolation policies:

```bash
curl -X POST http://127.0.0.1:8080/network/isolate \
  -H "Content-Type: application/json" \
  -d '{
    "name": "block-api-worker",
    "sourceNamespace": "default",
    "sourceSelector": {"app": "api"},
    "targetNamespace": "default",
    "targetSelector": {"app": "worker"}
  }'

kubectl get networkpolicies
kubectl describe networkpolicy block-api-worker-source
kubectl describe networkpolicy block-api-worker-target
```

Verify traffic after isolation:

```bash
kubectl exec deployment/api -- wget -qO- -T 3 http://worker.default.svc.cluster.local:8080 || echo "traffic blocked as expected"
```

## Design Decisions

- The service remains one deployable Go process.
- `/healthz` checks process liveness; `/readyz` checks Kubernetes dependency readiness.
- Deployment health uses `Status.AvailableReplicas`.
- Network isolation uses simple default-deny ingress and egress policies for selected workloads.
- A dedicated ServiceAccount and cluster-wide RBAC are used because the service reads Deployments across namespaces and creates NetworkPolicies in workload namespaces.
- Tests use fake Kubernetes clients so most behavior is verified without a live cluster.
