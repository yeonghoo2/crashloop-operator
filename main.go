// Package main implements a Kubernetes operator that monitors ReplicaSets
// and automatically deletes those in CrashLoopBackOff state.
package main

import (
	"context"
	"flag"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReplicaSetController reconciles ReplicaSet objects
type ReplicaSetController struct {
	client.Client
	Scheme    *runtime.Scheme
	clientset *kubernetes.Clientset
}

// Reconcile handles the reconciliation logic for ReplicaSets.
// It monitors ReplicaSets and deletes those that are in CrashLoopBackOff state
// with 1 replica and 0 ready replicas.
func (r *ReplicaSetController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// ReplicaSet 조회
	var rs appsv1.ReplicaSet
	if err := r.Get(ctx, req.NamespacedName, &rs); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "ReplicaSet을 조회할 수 없습니다")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// 조건 확인: replicas가 1이고 ready replicas가 0인지
	if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 1 && rs.Status.ReadyReplicas == 0 {
		// 해당 ReplicaSet의 Pod들을 조회
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(rs.Namespace),
			client.MatchingLabels(rs.Spec.Selector.MatchLabels),
		}

		if err := r.List(ctx, podList, listOpts...); err != nil {
			log.Error(err, "Pod 목록을 조회할 수 없습니다")
			return reconcile.Result{}, err
		}

		// Pod가 CrashLoopBackOff 상태인지 확인
		for _, pod := range podList.Items {
			if isPodInCrashLoopBackOff(&pod) {
				log.Info("CrashLoopBackOff 상태의 ReplicaSet을 삭제합니다",
					"replicaset", rs.Name,
					"namespace", rs.Namespace,
					"pod", pod.Name)

				// ReplicaSet 삭제
				if err := r.Delete(ctx, &rs); err != nil {
					log.Error(err, "ReplicaSet 삭제에 실패했습니다")
					return reconcile.Result{}, err
				}

				log.Info("ReplicaSet이 성공적으로 삭제되었습니다", "replicaset", rs.Name)
				return reconcile.Result{}, nil
			}
		}
	}

	// 30초 후 다시 체크
	return reconcile.Result{RequeueAfter: time.Second * 30}, nil
}

// isPodInCrashLoopBackOff checks if a pod is in CrashLoopBackOff state
// by examining container status and restart count.
func isPodInCrashLoopBackOff(pod *corev1.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
				return true
			}
		}
		// RestartCount가 높고 Ready가 false인 경우도 체크
		if containerStatus.RestartCount > 3 && !containerStatus.Ready {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReplicaSetController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.ReplicaSet{}).
		Complete(r)
}

func main() {
	var kubeconfig string
	var masterURL string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig 파일 경로")
	flag.StringVar(&masterURL, "master", "", "Kubernetes API server URL")
	flag.Parse()

	klog.InitFlags(nil)

	// Kubernetes 설정 로드
	cfg, err := getConfig(kubeconfig, masterURL)
	if err != nil {
		klog.Fatalf("Kubernetes 설정을 로드할 수 없습니다: %v", err)
	}

	// Controller Manager 생성
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: runtime.NewScheme(),
	})
	if err != nil {
		klog.Fatalf("Manager 생성에 실패했습니다: %v", err)
	}

	// Scheme에 타입 등록
	if err := appsv1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("Scheme 등록에 실패했습니다: %v", err)
	}
	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("Scheme 등록에 실패했습니다: %v", err)
	}

	// Kubernetes clientset 생성
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Clientset 생성에 실패했습니다: %v", err)
	}

	// Controller 등록
	if err = (&ReplicaSetController{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		klog.Fatalf("Controller 설정에 실패했습니다: %v", err)
	}

	klog.Info("ReplicaSet Operator를 시작합니다...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("Manager 시작에 실패했습니다: %v", err)
	}
}

// getConfig creates a Kubernetes client configuration from kubeconfig file or in-cluster config.
func getConfig(kubeconfig, masterURL string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	}
	return rest.InClusterConfig()
}
