package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/orinameh/aegis/internal/config"
)

// PodSummary is a read-only snapshot of a Kubernetes pod.
type PodSummary struct {
	Name      string
	Namespace string
	Phase     string
	Created   time.Time
}

// JobSummary is a read-only snapshot of a Kubernetes job.
type JobSummary struct {
	Name      string
	Namespace string
	Succeeded int32
	Failed    int32
	Created   time.Time
	Finished  time.Time
}

// PVCSummary is a read-only snapshot of a Kubernetes persistent volume claim.
type PVCSummary struct {
	Name      string
	Namespace string
	Phase     string
}

// Inventory is a read-only snapshot of Kubernetes resources across the namespaces
// selected by the sweeper's config.
type Inventory struct {
	Pods []PodSummary
	Jobs []JobSummary
	PVCs []PVCSummary
}

// List returns a read-only inventory of Kubernetes resources for the namespaces
// selected by the given config. It performs no mutations.
func (s *Sweeper) List(ctx context.Context, cfg *config.K8sConfig) (*Inventory, error) {
	namespaces, err := s.getNamespaces(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespaces: %w", err)
	}

	var inv Inventory

	for _, ns := range namespaces {
		pods, err := s.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods in %q: %w", ns, err)
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			inv.Pods = append(inv.Pods, PodSummary{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Phase:     string(pod.Status.Phase),
				Created:   pod.CreationTimestamp.Time,
			})
		}

		jobs, err := s.clientset.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list jobs in %q: %w", ns, err)
		}
		for i := range jobs.Items {
			job := &jobs.Items[i]
			inv.Jobs = append(inv.Jobs, JobSummary{
				Name:      job.Name,
				Namespace: job.Namespace,
				Succeeded: job.Status.Succeeded,
				Failed:    job.Status.Failed,
				Created:   job.CreationTimestamp.Time,
				Finished:  jobFinishTime(job),
			})
		}

		pvcs, err := s.clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list PVCs in %q: %w", ns, err)
		}
		for i := range pvcs.Items {
			pvc := &pvcs.Items[i]
			inv.PVCs = append(inv.PVCs, PVCSummary{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
				Phase:     string(pvc.Status.Phase),
			})
		}
	}

	return &inv, nil
}
