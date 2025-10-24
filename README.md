# CrashLoop Operator

[![GitHub Release](https://img.shields.io/github/v/release/yeonghoo2/crashloop-operator)](https://github.com/yeonghoo2/crashloop-operator/releases)
[![Docker Image](https://img.shields.io/docker/v/yeonghoo2/crashloop-operator?label=docker)](https://ghcr.io/yeonghoo2/crashloop-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/yeonghoo2/crashloop-operator)](https://goreportcard.com/report/github.com/yeonghoo2/crashloop-operator)
[![License](https://img.shields.io/github/license/yeonghoo2/crashloop-operator)](LICENSE)
[![Helm Chart](https://img.shields.io/badge/helm-chart-blue)](https://yeonghoo2.github.io/crashloop-operator)

> A Kubernetes operator that automatically removes ReplicaSets where ALL pods are in CrashLoopBackOff state.

## Overview

The CrashLoop Operator automatically detects and removes ReplicaSets where all pods are stuck in CrashLoopBackOff state. It's particularly useful for Argo Rollouts environments where failed deployments can leave ReplicaSets running indefinitely.

### Key Features

- **🎯 Precise Targeting**: Only deletes ReplicaSets where ALL pods are in CrashLoopBackOff
- **🚀 Argo Rollouts Ready**: Default configuration targets Argo Rollouts ReplicaSets
- **⚡ Automatic Cleanup**: Removes problematic ReplicaSets automatically
- **🛡️ Safe**: Conservative approach - only deletes when all pods are clearly failing
- **📊 Configurable**: Customizable restart count and check intervals

## Quick Start

### Installation

```bash
helm repo add crashloop-operator https://yeonghoo2.github.io/crashloop-operator
helm repo update
helm install crashloop-operator crashloop-operator/crashloop-operator
```

### Verification

```bash
kubectl get pods -l app.kubernetes.io/name=crashloop-operator
kubectl logs -l app.kubernetes.io/name=crashloop-operator -f
```

## How It Works

1. **Monitor**: Watches ReplicaSets across the cluster
2. **Analyze**: Identifies ReplicaSets where ALL pods are in CrashLoopBackOff
3. **Act**: Safely removes qualifying ReplicaSets
4. **Repeat**: Rechecks every 30 seconds

### Deletion Criteria

A ReplicaSet is deleted when:
- Matches target labels (default: Argo Rollouts ReplicaSets)
- Has at least one pod
- **ALL pods** are in CrashLoopBackOff state

#### CrashLoopBackOff Detection

A pod is in CrashLoopBackOff if:
- Container has `state.waiting.reason == "CrashLoopBackOff"` OR
- Container has `restartCount > minRestartCount` (default: 3) AND `ready == false`

## Configuration

### Default Behavior

By default, the operator targets ReplicaSets with the `rollouts-pod-template-hash` label (Argo Rollouts ReplicaSets). To target all ReplicaSets, set `targetLabels: {}`.

### Helm Values

```yaml
operator:
  targetLabels:                  # Target specific ReplicaSets
    rollouts-pod-template-hash: ""  # Default: Argo Rollouts
  logLevel: 2                    # Log verbosity (0-4)
  recheckInterval: 30            # Check interval in seconds
  minRestartCount: 3             # Minimum restart count threshold
  watchNamespace: ""             # Watch specific namespace (empty = all)
  healthPort: 8081              # Health check port
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TARGET_LABELS` | `{"rollouts-pod-template-hash": ""}` | JSON map of labels to target |
| `MIN_RESTART_COUNT` | `3` | Minimum restart count threshold |
| `RECHECK_INTERVAL` | `30` | Check interval in seconds |
| `WATCH_NAMESPACE` | `""` | Specific namespace to watch |

## Testing

Create a test ReplicaSet that will crash:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: test-crashloop-rs
  labels:
    app: test-crashloop
    rollouts-pod-template-hash: "test123"  # Argo Rollouts label
spec:
  replicas: 2
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
        command: ["sh", "-c", "echo 'Starting...'; sleep 2; exit 1"]
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
      restartPolicy: Always
EOF
```

Monitor the deletion:

```bash
watch kubectl get rs,pods -l app=test-crashloop
```

The operator should delete the ReplicaSet within 30 seconds after ALL pods enter CrashLoopBackOff state.

## Development

```bash
git clone https://github.com/yeonghoo2/crashloop-operator.git
cd crashloop-operator

# Build and run
make build
make run

# Build Docker image
make docker-build
```

## Troubleshooting

**Check operator status:**
```bash
kubectl get pods -l app.kubernetes.io/name=crashloop-operator
kubectl logs -l app.kubernetes.io/name=crashloop-operator
```

**Enable debug logging:**
```bash
helm upgrade crashloop-operator crashloop-operator/crashloop-operator \
  --set operator.logLevel=4
```

**Verify RBAC:**
```bash
kubectl get clusterrole,clusterrolebinding | grep crashloop-operator
```

## Important Notes

- **Argo Rollouts**: Default configuration targets Argo Rollouts ReplicaSets
- **Safety**: Only deletes ReplicaSets where ALL pods are in CrashLoopBackOff
- **Production**: Test thoroughly before deploying to production
- **Permissions**: Requires cluster-wide permissions to view and delete ReplicaSets

## License

MIT License - see [LICENSE](LICENSE) file for details.