# tyk-sre-assignment

This repository contains the boilerplate projects for the SRE role interview assignments.

### Project

Location: https://github.com/TykTechnologies/tyk-sre-assignment/tree/main/golang

In order to build the project run:
```
go mod tidy & go build
```

To run it against a real Kubernetes API server:
```
./tyk-sre-assignment --kubeconfig '/path/to/your/kube/conf' --address ":8080"
```

To execute unit tests:
```
go test -v
```

## Additional Setup [Stories 1-5]

The sections below document the completed assignment implementation while keeping
the original project instructions above unchanged.

### Local Go Checks [Stories 1 and 3]

From the repository root:

```bash
cd golang
go test ./...
go build -o tyk-sre-assignment .
```

The service exposes these endpoints:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl http://127.0.0.1:8080/deployments/health
```

`/healthz` confirms that the process is serving requests. `/readyz` checks
Kubernetes API connectivity, and `/deployments/health` reports Deployment
replica health across namespaces.

### Container Image [Story 4]

Build the image from the repository root:

```bash
docker build -t tyk-sre-assignment:local .
```

Docker builds the application in one stage, then copies only the executable into
a smaller final image. The application runs as a non-root user.

### GitHub Actions [Story 4]

The workflow runs Go tests, builds the binary, and builds the Docker image on pull
requests and pushes to `main`. It validates the image but does not publish it to a
registry.

### Helm Deployment [Story 5]

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

The chart creates the application Deployment, Service, ServiceAccount, and the
RBAC permissions needed to read Deployments and create NetworkPolicies.

### Minikube Demonstration [Stories 1-5]

For a fresh Minikube cluster with NetworkPolicy enforcement, use Calico:

```bash
minikube start --driver=docker --cni=calico
docker build -t tyk-sre-assignment:local .
minikube image load tyk-sre-assignment:local
```

If the image is rebuilt outside Minikube, repeat `minikube image load` so the
cluster receives the new image. Then install the chart using the Helm command
above. A running cluster can be accessed from the host with:

```bash
kubectl port-forward service/tyk-sre-service 8080:8080
```

Alternatively, build directly inside Minikube's Docker environment:

```bash
eval "$(minikube docker-env)"
docker build -t tyk-sre-assignment:local .
```

With this approach, `minikube image load` is not needed because the image is
already built in the Docker environment used by Minikube.

In another terminal, create the two test workloads and verify their initial
connectivity:

```bash
kubectl apply -f examples/test-workloads.yaml
kubectl rollout status deployment/api
kubectl rollout status deployment/worker
kubectl exec deployment/api -- wget -qO- http://worker.default.svc.cluster.local:8080
```

### Network Isolation [Story 2]

Create isolation between them through the Go service:

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

Inspect the policies and verify that the request is blocked:

```bash
kubectl get networkpolicies
kubectl describe networkpolicy block-api-worker-source
kubectl describe networkpolicy block-api-worker-target
kubectl exec deployment/api -- wget -qO- -T 3 http://worker.default.svc.cluster.local:8080 || echo "traffic blocked as expected"
```

The isolation endpoint creates default-deny ingress and egress policies for the
selected workloads. A CNI that enforces NetworkPolicy, such as Calico, is needed
for the traffic block to be observable in a local Minikube demonstration.
