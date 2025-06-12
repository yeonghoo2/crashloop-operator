# CrashLoop Operator Makefile

# Image configuration
IMG ?= ghcr.io/yeonghoo2/crashloop-operator:latest
REGISTRY ?= ghcr.io
IMAGE_NAME ?= yeonghoo2/crashloop-operator

# Go configuration
GO_VERSION ?= 1.21
GOOS ?= linux
GOARCH ?= amd64

.PHONY: help
help: ## Display available commands
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: fmt
fmt: ## Format Go code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests
	go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: tidy fmt vet ## Build binary
	mkdir -p bin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -a -o bin/manager main.go

.PHONY: run
run: fmt vet ## Run locally
	go run ./main.go

.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push Docker image
	docker push $(IMG)

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Build and push Docker image

##@ Helm

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	helm lint charts/crashloop-operator

.PHONY: helm-template
helm-template: ## Render Helm templates
	helm template crashloop-operator charts/crashloop-operator

.PHONY: helm-install
helm-install: ## Install with Helm
	helm install crashloop-operator charts/crashloop-operator

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade Helm release
	helm upgrade crashloop-operator charts/crashloop-operator

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm release
	helm uninstall crashloop-operator

.PHONY: helm-package
helm-package: ## Package Helm chart
	mkdir -p .deploy
	helm package charts/crashloop-operator --destination .deploy

##@ Testing

.PHONY: create-test-rs
create-test-rs: ## Create test ReplicaSet (triggers CrashLoopBackOff)
	@echo "Creating test ReplicaSet..."
	@kubectl apply -f - <<EOF || true
	apiVersion: apps/v1
	kind: ReplicaSet
	metadata:
	  name: test-crashloop-rs
	  namespace: default
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
	          limits:
	            cpu: 200m
	            memory: 128Mi
	      restartPolicy: Always
	EOF

.PHONY: delete-test-rs
delete-test-rs: ## Delete test ReplicaSet
	kubectl delete rs test-crashloop-rs --ignore-not-found=true

.PHONY: logs
logs: ## View operator logs
	kubectl logs -l app.kubernetes.io/name=crashloop-operator -f

.PHONY: status
status: ## Check current status
	@echo "=== Operator Status ==="
	kubectl get pods -l app.kubernetes.io/name=crashloop-operator
	@echo ""
	@echo "=== ReplicaSets Status ==="
	kubectl get rs
	@echo ""
	@echo "=== Pods Status ==="
	kubectl get pods

##@ Git & Release

.PHONY: git-status
git-status: ## Check Git status
	@echo "=== Git Status ==="
	git status
	@echo ""
	@echo "=== Important Files Existence ==="
	@ls -la go.mod go.sum main.go Dockerfile 2>/dev/null || echo "Some files are missing!"
	@echo ""
	@echo "=== go.sum File Size ==="
	@wc -l go.sum 2>/dev/null || echo "go.sum does not exist!"

.PHONY: prepare-release
prepare-release: tidy fmt vet ## Prepare for release
	@echo "=== Preparing Release ==="
	@echo "1. Tidying Go modules..."
	go mod tidy
	@echo "2. Formatting code..."
	go fmt ./...
	@echo "3. Vetting code..."
	go vet ./...
	@echo "4. Testing build..."
	go build -o /tmp/test-manager main.go
	rm -f /tmp/test-manager
	@echo "5. Linting Helm chart..."
	helm lint charts/crashloop-operator
	@echo "✅ Release preparation complete!"

.PHONY: create-tag
create-tag: prepare-release ## Create tag (use VERSION=v0.1.0 format)
ifndef VERSION
	$(error VERSION is required. Usage: make create-tag VERSION=v0.1.0)
endif
	@echo "=== Creating tag $(VERSION) ==="
	git add .
	git commit -m "prepare release $(VERSION)" || true
	git push origin main
	git tag $(VERSION)
	git push origin $(VERSION)
	@echo "✅ Tag $(VERSION) created successfully!"

.PHONY: delete-tag
delete-tag: ## Delete tag (use VERSION=v0.1.0 format)
ifndef VERSION
	$(error VERSION is required. Usage: make delete-tag VERSION=v0.1.0)
endif
	@echo "=== Deleting tag $(VERSION) ==="
	git tag -d $(VERSION) || true
	git push --delete origin $(VERSION) || true
	@echo "✅ Tag $(VERSION) deleted successfully!"

##@ Cleanup

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf .deploy/
	rm -f cover.out

.PHONY: clean-all
clean-all: clean helm-uninstall delete-test-rs ## Clean everything