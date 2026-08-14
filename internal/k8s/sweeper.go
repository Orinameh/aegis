package k8s

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"aegis/internal/config"
	"aegis/internal/guard"
)

// Sweeper handles Kubernetes cleanup operations
type Sweeper struct {
	clientset  *kubernetes.Clientset
	httpClient *http.Client
	logger     *zap.Logger
	guard      *guard.Guard
}

// NewSweeper creates a new Kubernetes sweeper instance
func NewSweeper(logger *zap.Logger, guard *guard.Guard) (*Sweeper, error) {
	// Try in-cluster config first.
	//
	// BUG FIX: this variable was previously named `config`, which shadows
	// the imported `config` package for the rest of this function. It
	// happened not to break anything today only because nothing inside
	// this function referenced config.SomeType afterward — a latent
	// footgun. Renamed to restConfig to make the shadowing impossible.
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		if homedir.HomeDir() != "" {
			loadingRules.ExplicitPath = clientcmd.RecommendedHomeFile
		}

		kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			&clientcmd.ConfigOverrides{},
		)

		restConfig, err = kubeconfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create Kubernetes client config: %w", err)
		}
	}

	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes HTTP client: %w", err)
	}

	clientset, err := kubernetes.NewForConfigAndClient(restConfig, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return &Sweeper{
		clientset:  clientset,
		httpClient: httpClient,
		logger:     logger,
		guard:      guard,
	}, nil
}

// Close releases the underlying HTTP connections held by the sweeper.
func (s *Sweeper) Close() error {
	if s.httpClient != nil {
		s.httpClient.CloseIdleConnections()
	}
	return nil
}

// Sweep executes the Kubernetes cleanup operations
func (s *Sweeper) Sweep(ctx context.Context, cfg *config.K8sConfig) error {
	s.logger.Info("starting Kubernetes sweeper")

	// Get namespaces to process
	namespaces, err := s.getNamespaces(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get namespaces: %w", err)
	}

	totalResourcesRemoved := 0

	for _, namespace := range namespaces {
		s.logger.Debug("processing namespace", zap.String("namespace", namespace))

		// Clean up pods
		if cfg.DeleteFailedPods || cfg.DeleteEvictedPods {
			removed, err := s.cleanupPods(ctx, namespace, cfg)
			if err != nil {
				s.logger.Error("failed to cleanup pods",
					zap.String("namespace", namespace),
					zap.Error(err),
				)
				continue
			}
			totalResourcesRemoved += removed
		}

		// Clean up jobs
		if cfg.DeleteCompletedJobs || cfg.DeleteSucceededJobs {
			removed, err := s.cleanupJobs(ctx, namespace, cfg)
			if err != nil {
				s.logger.Error("failed to cleanup jobs",
					zap.String("namespace", namespace),
					zap.Error(err),
				)
				continue
			}
			totalResourcesRemoved += removed
		}

		// Clean up orphaned PVCs
		if cfg.DeleteOrphanedPVCs {
			removed, err := s.cleanupPVCs(ctx, namespace)
			if err != nil {
				s.logger.Error("failed to cleanup PVCs",
					zap.String("namespace", namespace),
					zap.Error(err),
				)
				continue
			}
			totalResourcesRemoved += removed
		}
	}

	s.logger.Info("Kubernetes sweeper completed",
		zap.Int("total_resources_removed", totalResourcesRemoved),
	)

	return nil
}

// getNamespaces returns list of namespaces to process
func (s *Sweeper) getNamespaces(ctx context.Context, cfg *config.K8sConfig) ([]string, error) {
	if len(cfg.IncludeNamespaces) > 0 {
		return cfg.IncludeNamespaces, nil
	}

	namespaceList, err := s.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	excludeSet := make(map[string]bool)
	for _, ns := range cfg.ExcludeNamespaces {
		excludeSet[ns] = true
	}

	var namespaces []string
	for _, ns := range namespaceList.Items {
		if !excludeSet[ns.Name] {
			namespaces = append(namespaces, ns.Name)
		}
	}

	return namespaces, nil
}

// cleanupPods removes failed and evicted pods
func (s *Sweeper) cleanupPods(ctx context.Context, namespace string, cfg *config.K8sConfig) (int, error) {
	podList, err := s.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to list pods: %w", err)
	}

	removed := 0
	for i := range podList.Items {
		// Index into the slice directly (rather than range-copying into
		// `pod`) so &pod is always the address of a distinct value, not a
		// reused loop variable — harmless under Go 1.22+'s per-iteration
		// loop vars, but avoids relying on that toolchain behavior.
		pod := &podList.Items[i]

		if s.shouldDeletePod(pod, cfg) {
			resource := &guard.Resource{
				Type:      guard.ResourcePod,
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Labels:    pod.Labels,
				Metadata: map[string]any{
					"phase":      string(pod.Status.Phase),
					"reason":     pod.Status.Reason,
					"owner_refs": pod.OwnerReferences,
					"created":    pod.CreationTimestamp.Time,
				},
			}

			err := s.guard.CheckAndExecute(ctx, resource, "delete", func() error {
				if err := s.clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
					return fmt.Errorf("failed to delete pod: %w", err)
				}
				removed++
				s.logger.Debug("deleted pod",
					zap.String("pod", pod.Name),
					zap.String("namespace", namespace),
					zap.String("phase", string(pod.Status.Phase)),
				)
				return nil
			})

			if err != nil {
				if !isProtectionDenial(err) {
					s.logger.Error("failed to delete pod",
						zap.String("pod", pod.Name),
						zap.String("namespace", namespace),
						zap.Error(err),
					)
				} else {
					s.logger.Debug("pod protected, skipping",
						zap.String("pod", pod.Name),
						zap.String("namespace", namespace),
						zap.Error(err),
					)
				}
			}
		}
	}

	return removed, nil
}

// shouldDeletePod determines if a pod should be deleted
func (s *Sweeper) shouldDeletePod(pod *corev1.Pod, cfg *config.K8sConfig) bool {
	// BUG FIX: evicted pods are NOT reported with phase PodUnknown — that
	// phase means the node stopped reporting status entirely (e.g. lost
	// kubelet heartbeat), a completely different failure mode. A real
	// eviction sets phase=Failed with status.reason=="Evicted". The
	// original code checked the eviction case under `case PodUnknown`,
	// so it would essentially never match a genuinely evicted pod, and
	// evicted pods would instead just fall into the PodFailed branch
	// below (deleted only if they have no Job owner, regardless of
	// whether DeleteEvictedPods was even enabled).
	if pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted" {
		return cfg.DeleteEvictedPods
	}

	switch pod.Status.Phase {
	case corev1.PodFailed:
		if !cfg.DeleteFailedPods {
			return false
		}
		// Don't delete job pods unless they're completed — job pods are
		// handled by cleanupJobs instead.
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "Job" {
				return false
			}
		}
		return true

	default:
		return false
	}
}

// cleanupJobs removes completed jobs.
//
// DeleteSucceededJobs removes jobs that finished successfully.
// DeleteCompletedJobs removes jobs that have finished at all — success
// OR failure (exhausted retries / backoffLimit).
//
// BUG FIX: the original code checked the *identical* condition
// (CompletionTime != nil && Succeeded > 0) for both flags, which meant
// DeleteCompletedJobs and DeleteSucceededJobs did the same thing and
// failed jobs were never cleaned up under either flag. The unused
// `batchv1` import was the tell — it was meant for exactly this: reading
// the Job's condition list to distinguish Complete from Failed.
func (s *Sweeper) cleanupJobs(ctx context.Context, namespace string, cfg *config.K8sConfig) (int, error) {
	jobList, err := s.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to list jobs: %w", err)
	}

	removed := 0
	retentionCutoff := time.Now().AddDate(0, 0, -cfg.JobRetentionDays)

	for i := range jobList.Items {
		job := &jobList.Items[i]

		succeeded := job.Status.CompletionTime != nil && job.Status.Succeeded > 0
		failed := jobConditionTrue(job, batchv1.JobFailed)

		shouldDelete := false
		if cfg.DeleteSucceededJobs && succeeded {
			shouldDelete = true
		}
		if cfg.DeleteCompletedJobs && (succeeded || failed) {
			shouldDelete = true
		}
		if !succeeded && !failed {
			// Still running, or status not yet observed.
			shouldDelete = false
		}

		// Respect the retention window, keyed off whichever timestamp we
		// have available (completion for success, condition transition
		// for failure).
		if shouldDelete {
			finishedAt := jobFinishTime(job)
			if !finishedAt.IsZero() && finishedAt.After(retentionCutoff) {
				shouldDelete = false
			}
		}

		if !shouldDelete {
			continue
		}

		resource := &guard.Resource{
			Type:      guard.ResourceJob,
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    job.Labels,
			Metadata: map[string]any{
				"succeeded": job.Status.Succeeded,
				"failed":    job.Status.Failed,
				"created":   job.CreationTimestamp.Time,
				"finished":  jobFinishTime(job),
			},
		}

		err := s.guard.CheckAndExecute(ctx, resource, "delete", func() error {
			if err := s.clientset.BatchV1().Jobs(namespace).Delete(ctx, job.Name, metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete job: %w", err)
			}
			removed++
			s.logger.Debug("deleted job",
				zap.String("job", job.Name),
				zap.String("namespace", namespace),
			)
			return nil
		})

		if err != nil {
			if !isProtectionDenial(err) {
				s.logger.Error("failed to delete job",
					zap.String("job", job.Name),
					zap.String("namespace", namespace),
					zap.Error(err),
				)
			} else {
				s.logger.Debug("job protected, skipping",
					zap.String("job", job.Name),
					zap.String("namespace", namespace),
					zap.Error(err),
				)
			}
		}
	}

	return removed, nil
}

// jobConditionTrue reports whether the given Job has the given condition
// type set to ConditionTrue.
func jobConditionTrue(job *batchv1.Job, condType batchv1.JobConditionType) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == condType && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// jobFinishTime returns the best available "this job is done" timestamp:
// CompletionTime for successful jobs, or the JobFailed condition's
// transition time for failed ones. Returns the zero Time if neither is
// available (job still running / status not yet observed).
func jobFinishTime(job *batchv1.Job) time.Time {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Time
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return c.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// cleanupPVCs removes orphaned PVCs
func (s *Sweeper) cleanupPVCs(ctx context.Context, namespace string) (int, error) {
	pvcList, err := s.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to list PVCs: %w", err)
	}

	removed := 0
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]

		// Check if PVC is not bound to any pod.
		// This is a simple check - a more sophisticated implementation would check pod references.
		if pvc.Status.Phase != corev1.ClaimLost {
			continue
		}

		resource := &guard.Resource{
			Type:      guard.ResourcePVC,
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Labels:    pvc.Labels,
			Metadata: map[string]any{
				"status": string(pvc.Status.Phase),
				"size":   pvc.Spec.Resources.Requests.Storage(),
			},
		}

		err := s.guard.CheckAndExecute(ctx, resource, "delete", func() error {
			if err := s.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete PVC: %w", err)
			}
			removed++
			s.logger.Debug("deleted orphaned PVC",
				zap.String("pvc", pvc.Name),
				zap.String("namespace", namespace),
			)
			return nil
		})

		if err != nil {
			if !isProtectionDenial(err) {
				s.logger.Error("failed to delete PVC",
					zap.String("pvc", pvc.Name),
					zap.String("namespace", namespace),
					zap.Error(err),
				)
			} else {
				s.logger.Debug("PVC protected, skipping",
					zap.String("pvc", pvc.Name),
					zap.String("namespace", namespace),
					zap.Error(err),
				)
			}
		}
	}

	return removed, nil
}

// isProtectionDenial checks if an error is a protection denial
func isProtectionDenial(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "deletion not approved") ||
		strings.Contains(msg, "not allowed") ||
		strings.Contains(msg, "critically protected") ||
		strings.Contains(msg, "strictly protected")
}
