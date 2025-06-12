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

	// Fetch the ReplicaSet
	var rs appsv1.ReplicaSet
	if err := r.Get(ctx, req.NamespacedName, &rs); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to get ReplicaSet")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Check conditions: 1 replica and 0 ready replicas
	if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 1 && rs.Status.ReadyReplicas == 0 {
		// List pods for this ReplicaSet
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(rs.Namespace),
			client.MatchingLabels(rs.Spec.Selector.MatchLabels),
		}

		if err := r.List(ctx, podList, listOpts...); err != nil {
			log.Error(err, "Failed to list pods")
			return reconcile.Result{}, err
		}

		// Check if any pod is in CrashLoopBackOff state
		for _, pod := range podList.Items {
			if isPodInCrashLoopBackOff(&pod) {
				log.Info("Deleting ReplicaSet with CrashLoopBackOff pods",
					"replicaset", rs.Name,
					"namespace", rs.Namespace,
					"pod", pod.Name)

				// Delete the ReplicaSet
				if err := r.Delete(ctx, &rs); err != nil {
					log.Error(err, "Failed to delete ReplicaSet")
					return reconcile.Result{}, err
				}

				log.Info("ReplicaSet successfully deleted", "replicaset", rs.Name)
				return reconcile.Result{}, nil
			}
		}
	}

	// Requeue after 30 seconds
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
		// Also check for high restart count with not ready status
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

	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	flag.StringVar(&masterURL, "master", "", "Kubernetes API server URL")
	flag.Parse()

	klog.InitFlags(nil)

	// Load Kubernetes configuration
	cfg, err := getConfig(kubeconfig, masterURL)
	if err != nil {
		klog.Fatalf("Failed to load Kubernetes configuration: %v", err)
	}

	// Create controller manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: runtime.NewScheme(),
	})
	if err != nil {
		klog.Fatalf("Failed to create manager: %v", err)
	}

	// Register types to scheme
	if err := appsv1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("Failed to add apps/v1 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("Failed to add core/v1 to scheme: %v", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to create clientset: %v", err)
	}

	// Register controller
	if err = (&ReplicaSetController{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		clientset: clientset,
	}).SetupWithManager(mgr); err != nil {
		klog.Fatalf("Failed to setup controller: %v", err)
	}

	klog.Info("Starting ReplicaSet Operator...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("Failed to start manager: %v", err)
	}
}

// getConfig creates a Kubernetes client configuration from kubeconfig file or in-cluster config.
func getConfig(kubeconfig, masterURL string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	}
	return rest.InClusterConfig()
}
