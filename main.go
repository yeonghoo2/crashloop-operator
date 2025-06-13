// Package main implements a Kubernetes operator that monitors ReplicaSets
// and automatically deletes those in CrashLoopBackOff state.
package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OperatorConfig holds the configuration for the operator
type OperatorConfig struct {
	TargetLabels    map[string]string
	MinRestartCount int32
	RecheckInterval time.Duration
	WatchNamespace  string
}

// ReplicaSetController reconciles ReplicaSet objects
type ReplicaSetController struct {
	client.Client
	Scheme    *runtime.Scheme
	clientset *kubernetes.Clientset
	Config    *OperatorConfig
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

	// Check if ReplicaSet matches target labels
	if !r.matchesTargetLabels(&rs) {
		log.V(4).Info("ReplicaSet does not match target labels, skipping",
			"replicaset", rs.Name,
			"namespace", rs.Namespace,
			"labels", rs.Labels)
		return reconcile.Result{RequeueAfter: r.Config.RecheckInterval}, nil
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
			if r.isPodInCrashLoopBackOff(&pod) {
				log.Info("Deleting ReplicaSet with CrashLoopBackOff pods",
					"replicaset", rs.Name,
					"namespace", rs.Namespace,
					"pod", pod.Name,
					"matchedLabels", r.getMatchedLabels(&rs))

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

	// Requeue after configured interval
	return reconcile.Result{RequeueAfter: r.Config.RecheckInterval}, nil
}

// matchesTargetLabels checks if the ReplicaSet has the required target labels
func (r *ReplicaSetController) matchesTargetLabels(rs *appsv1.ReplicaSet) bool {
	// If no target labels configured, match all ReplicaSets
	if len(r.Config.TargetLabels) == 0 {
		return true
	}

	// Check if ReplicaSet has all required labels
	for key, value := range r.Config.TargetLabels {
		rsValue, exists := rs.Labels[key]
		if !exists {
			return false
		}
		// If target value is empty string, just check label existence
		// Otherwise, check for exact match
		if value != "" && rsValue != value {
			return false
		}
	}
	return true
}

// getMatchedLabels returns the labels that matched the target criteria
func (r *ReplicaSetController) getMatchedLabels(rs *appsv1.ReplicaSet) map[string]string {
	matched := make(map[string]string)
	for key := range r.Config.TargetLabels {
		if value, exists := rs.Labels[key]; exists {
			matched[key] = value
		}
	}
	return matched
}

// isPodInCrashLoopBackOff checks if a pod is in CrashLoopBackOff state
// by examining container status and restart count.
func (r *ReplicaSetController) isPodInCrashLoopBackOff(pod *corev1.Pod) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
				return true
			}
		}
		// Also check for high restart count with not ready status
		if containerStatus.RestartCount > r.Config.MinRestartCount && !containerStatus.Ready {
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
	// Initialize klog and controller-runtime flags
	ctrl.SetLogger(klog.Background())

	// Load configuration from environment variables
	config := loadConfig()

	// Load Kubernetes configuration (use in-cluster config by default)
	cfg, err := ctrl.GetConfig()
	if err != nil {
		klog.Fatalf("Failed to load Kubernetes configuration: %v", err)
	}

	// Create controller manager with health probe configuration
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 runtime.NewScheme(),
		LeaderElection:         false, // Disabled for single replica deployment
		HealthProbeBindAddress: ":8081",
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
		Config:    config,
	}).SetupWithManager(mgr); err != nil {
		klog.Fatalf("Failed to setup controller: %v", err)
	}

	// Add health and ready checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		klog.Fatalf("Failed to add health check: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		klog.Fatalf("Failed to add ready check: %v", err)
	}

	// Start the manager
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("Failed to start manager: %v", err)
	}
}

// loadConfig loads configuration from environment variables
func loadConfig() *OperatorConfig {
	config := &OperatorConfig{
		TargetLabels:    make(map[string]string),
		MinRestartCount: 3,
		RecheckInterval: 30 * time.Second,
	}

	// Load target labels from environment variable
	if targetLabelsEnv := os.Getenv("TARGET_LABELS"); targetLabelsEnv != "" {
		if err := json.Unmarshal([]byte(targetLabelsEnv), &config.TargetLabels); err != nil {
			klog.Warningf("Failed to parse TARGET_LABELS: %v, using empty map", err)
		}
	}

	// Load other configuration values
	if minRestartCountEnv := os.Getenv("MIN_RESTART_COUNT"); minRestartCountEnv != "" {
		if count, err := strconv.ParseInt(minRestartCountEnv, 10, 32); err == nil {
			config.MinRestartCount = int32(count)
		}
	}

	if recheckIntervalEnv := os.Getenv("RECHECK_INTERVAL"); recheckIntervalEnv != "" {
		if interval, err := strconv.ParseInt(recheckIntervalEnv, 10, 64); err == nil {
			config.RecheckInterval = time.Duration(interval) * time.Second
		}
	}

	if watchNamespaceEnv := os.Getenv("WATCH_NAMESPACE"); watchNamespaceEnv != "" {
		config.WatchNamespace = watchNamespaceEnv
	}

	return config
}