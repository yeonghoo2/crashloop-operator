# CrashLoop Operator Makefile

# 이미지 정보
IMG ?= ghcr.io/yeonghoo2/crashloop-operator:latest
REGISTRY ?= ghcr.io
IMAGE_NAME ?= yeonghoo2/crashloop-operator

# Go 관련 변수
GO_VERSION ?= 1.21
GOOS ?= linux
GOARCH ?= amd64

.PHONY: help
help: ## 사용 가능한 명령어 표시
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $1, $2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: tidy
tidy: ## Go mod tidy 실행
	go mod tidy

.PHONY: fmt
fmt: ## Go 코드 포맷팅
	go fmt ./...

.PHONY: vet
vet: ## Go vet 실행
	go vet ./...

.PHONY: test
test: fmt vet ## 테스트 실행
	go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: tidy fmt vet ## 바이너리 빌드
	mkdir -p bin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -a -o bin/manager main.go

.PHONY: run
run: fmt vet ## 로컬에서 실행
	go run ./main.go

.PHONY: docker-build
docker-build: ## Docker 이미지 빌드
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Docker 이미지 푸시
	docker push $(IMG)

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Docker 이미지 빌드 및 푸시

##@ Helm

.PHONY: helm-lint
helm-lint: ## Helm Chart 검증
	helm lint charts/crashloop-operator

.PHONY: helm-template
helm-template: ## Helm 템플릿 확인
	helm template crashloop-operator charts/crashloop-operator

.PHONY: helm-install
helm-install: ## Helm으로 설치
	helm install crashloop-operator charts/crashloop-operator

.PHONY: helm-upgrade
helm-upgrade: ## Helm 업그레이드
	helm upgrade crashloop-operator charts/crashloop-operator

.PHONY: helm-uninstall
helm-uninstall: ## Helm 제거
	helm uninstall crashloop-operator

.PHONY: helm-package
helm-package: ## Helm Chart 패키징
	mkdir -p .deploy
	helm package charts/crashloop-operator --destination .deploy

##@ Testing

.PHONY: create-test-rs
create-test-rs: ## 테스트용 ReplicaSet 생성 (CrashLoopBackOff 유발)
	@echo "테스트용 ReplicaSet 생성 중..."
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
delete-test-rs: ## 테스트용 ReplicaSet 삭제
	kubectl delete rs test-crashloop-rs --ignore-not-found=true

.PHONY: logs
logs: ## Operator 로그 확인
	kubectl logs -l app.kubernetes.io/name=crashloop-operator -f

.PHONY: status
status: ## 현재 상태 확인
	@echo "=== Operator 상태 ==="
	kubectl get pods -l app.kubernetes.io/name=crashloop-operator
	@echo ""
	@echo "=== ReplicaSets 상태 ==="
	kubectl get rs
	@echo ""
	@echo "=== Pods 상태 ==="
	kubectl get pods

##@ Git & Release

.PHONY: git-status
git-status: ## Git 상태 확인
	@echo "=== Git Status ==="
	git status
	@echo ""
	@echo "=== 중요 파일들 존재 여부 ==="
	@ls -la go.mod go.sum main.go Dockerfile 2>/dev/null || echo "일부 파일이 없습니다!"
	@echo ""
	@echo "=== go.sum 파일 크기 ==="
	@wc -l go.sum 2>/dev/null || echo "go.sum이 없습니다!"

.PHONY: prepare-release
prepare-release: tidy fmt vet ## 릴리즈 준비
	@echo "=== 릴리즈 준비 중 ==="
	@echo "1. Go 모듈 정리..."
	go mod tidy
	@echo "2. 코드 포맷팅..."
	go fmt ./...
	@echo "3. 코드 검증..."
	go vet ./...
	@echo "4. 빌드 테스트..."
	go build -o /tmp/test-manager main.go
	rm -f /tmp/test-manager
	@echo "5. Helm 검증..."
	helm lint charts/crashloop-operator
	@echo "✅ 릴리즈 준비 완료!"

.PHONY: create-tag
create-tag: prepare-release ## 태그 생성 (VERSION=v0.1.0 형태로 사용)
ifndef VERSION
	$(error VERSION is required. Usage: make create-tag VERSION=v0.1.0)
endif
	@echo "=== 태그 $(VERSION) 생성 중 ==="
	git add .
	git commit -m "prepare release $(VERSION)" || true
	git push origin main
	git tag $(VERSION)
	git push origin $(VERSION)
	@echo "✅ 태그 $(VERSION) 생성 완료!"

.PHONY: delete-tag
delete-tag: ## 태그 삭제 (VERSION=v0.1.0 형태로 사용)
ifndef VERSION
	$(error VERSION is required. Usage: make delete-tag VERSION=v0.1.0)
endif
	@echo "=== 태그 $(VERSION) 삭제 중 ==="
	git tag -d $(VERSION) || true
	git push --delete origin $(VERSION) || true
	@echo "✅ 태그 $(VERSION) 삭제 완료!"

##@ Cleanup

.PHONY: clean
clean: ## 빌드 파일 정리
	rm -rf bin/
	rm -rf .deploy/
	rm -f cover.out

.PHONY: clean-all
clean-all: clean helm-uninstall delete-test-rs ## 모든 것 정리