# CrashLoop Operator

[![GitHub Release](https://img.shields.io/github/v/release/yeonghoo2/crashloop-operator)](https://github.com/yeonghoo2/crashloop-operator/releases)
[![Docker Image](https://img.shields.io/docker/v/yeonghoo2/crashloop-operator?label=docker)](https://ghcr.io/yeonghoo2/crashloop-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/yeonghoo2/crashloop-operator)](https://goreportcard.com/report/github.com/yeonghoo2/crashloop-operator)
[![License](https://img.shields.io/github/license/yeonghoo2/crashloop-operator)](LICENSE)
[![Helm Chart](https://img.shields.io/badge/helm-chart-blue)](https://yeonghoo2.github.io/crashloop-operator)

> A Kubernetes operator that automatically detects and removes ReplicaSets with pods stuck in CrashLoopBackOff state, helping maintain cluster health and resource efficiency.

## 🎯 Overview

The CrashLoop Operator monitors your Kubernetes cluster for ReplicaSets that meet specific failure criteria and automatically removes them to prevent resource waste and improve cluster stability. It's designed to handle scenarios where pods continuously fail to start, consuming cluster resources without providing any value.

### Key Features

- **🔍 Intelligent Detection**: Monitors ReplicaSets with exactly 1 replica and 0 ready replicas
- **⚡ Automatic Cleanup**: Removes ReplicaSets when pods are in CrashLoopBackOff state
- **🛡️ Safety First**: Only targets specific failure patterns to avoid accidental deletions
- **📊 Configurable Thresholds**: Customizable restart count and check intervals
- **🚀 Easy Deployment**: Simple Helm chart installation
- **🔒 Secure**: Minimal RBAC permissions and non-root container execution

## 🚀 Quick Start

### Prerequisites

- Kubernetes 1.20+
- Helm 3.8+

### Installation

Add the Helm repository and install the operator:

```bash
helm repo add crashloop-operator https://yeonghoo2.github.io/crashloop-operator
helm repo update
helm install crashloop-operator crashloop-operator/crashloop-operator
```

That's it! The operator will start monitoring your cluster immediately.

### Verification

Check if the operator is running:

```bash
kubectl get pods -l app.kubernetes.io/name=crashloop-operator
```

View operator logs:

```bash
kubectl logs -l app.kubernetes.io/name=crashloop-operator -f
```

## 📖 How It Works

The CrashLoop Operator follows a simple but effective logic:

1. **Monitor**: Continuously watches ReplicaSet resources across the cluster
2. **Analyze**: Identifies ReplicaSets that match deletion criteria:
   - Has exactly 1 replica specified
   - Has 0 ready replicas
   - Contains pods in CrashLoopBackOff state OR with high restart counts (>3 by default)
3. **Act**: Safely removes qualifying ReplicaSets to free up cluster resources
4. **Repeat**: Rechecks every 30 seconds (configurable)

### Deletion Criteria

A ReplicaSet will be deleted if **ALL** of the following conditions are met:

- `spec.replicas == 1`
- `status.readyReplicas == 0`
- At least one pod has:
  - `state.waiting.reason == "CrashLoopBackOff"` OR
  - `restartCount > minRestartCount` (default: 3) AND `ready == false`

## ⚙️ Configuration

### Helm Values

Customize the operator behavior using Helm values:

```yaml
operator:
  logLevel: 2                    # Log verbosity (0-4)
  recheckInterval: 30            # Check interval in seconds
  minRestartCount: 3             # Minimum restart count threshold
  watchNamespace: ""             # Watch specific namespace (empty = all)
  healthPort: 8081              # Health check port

resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi

# Security settings
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  readOnlyRootFilesystem: true
```

### Example Installation with Custom Values

```bash
helm install crashloop-operator crashloop-operator/crashloop-operator \
  --set operator.logLevel=3 \
  --set operator.recheckInterval=60 \
  --set operator.minRestartCount=5 \
  --set resources.requests.cpu=20m
```

## 🧪 Testing

### Create a Test CrashLoop Scenario

To verify the operator works correctly, create a ReplicaSet that will crash:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: test-crashloop-rs
  labels:
    app: test-crashloop
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-crashloop
  template:
    metadata:
      labels:
        app: test-crashloop
    spec:
      containers:
      - name: crashloop-container
        image: busybox
        command: ["sh", "-c", "echo 'Starting...'; sleep 5; exit 1"]
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
      restartPolicy: Always
EOF
```

Monitor the ReplicaSet and pods:

```bash
# Watch for the automatic deletion
watch kubectl get rs,pods -l app=test-crashloop
```

The operator should detect and remove the ReplicaSet within 2-3 minutes after the pod enters CrashLoopBackOff state.

## 🛠️ Development

### Building from Source

```bash
# Clone the repository
git clone https://github.com/yeonghoo2/crashloop-operator.git
cd crashloop-operator

# Build binary
make build

# Run locally (requires kubeconfig)
make run

# Build Docker image
make docker-build

# Run tests
make test
```

### Project Structure

```
crashloop-operator/
├── main.go                     # Operator source code
├── go.mod                      # Go module dependencies
├── Dockerfile                  # Container image definition
├── Makefile                    # Build and development commands
├── charts/crashloop-operator/  # Helm chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
└── .github/workflows/          # CI/CD pipelines
```

## 🔧 Troubleshooting

### Common Issues

**Operator not starting:**
```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=crashloop-operator

# Check logs for errors
kubectl logs -l app.kubernetes.io/name=crashloop-operator
```

**RBAC permission issues:**
```bash
# Verify ClusterRole and ClusterRoleBinding
kubectl get clusterrole,clusterrolebinding | grep crashloop-operator
```

**ReplicaSets not being deleted:**
```bash
# Check if ReplicaSets match deletion criteria
kubectl get rs -o wide

# Increase log level for more details
helm upgrade crashloop-operator crashloop-operator/crashloop-operator \
  --set operator.logLevel=4
```

### Debug Mode

Enable verbose logging for troubleshooting:

```bash
helm upgrade crashloop-operator crashloop-operator/crashloop-operator \
  --set operator.logLevel=4
```

## 🚨 Important Notes

- **Production Usage**: Thoroughly test in non-production environments before deploying to production clusters
- **Permissions**: The operator requires cluster-wide permissions to view and delete ReplicaSets
- **Scope**: Only targets ReplicaSets with exactly 1 replica to minimize impact
- **Safety**: Always verify deletion criteria match your requirements

### Development Workflow

1. Fork the repository
2. Create a feature branch
3. Make changes and add tests
4. Run tests: `make test`
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
