package k8s

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/orinameh/aegis/internal/config"
)

func TestShouldDeletePod(t *testing.T) {
	newPod := func(phase corev1.PodPhase, reason string, ownerKinds ...string) *corev1.Pod {
		var owners []metav1.OwnerReference
		for _, kind := range ownerKinds {
			owners = append(owners, metav1.OwnerReference{Kind: kind})
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "pod",
				Namespace:       "ns",
				OwnerReferences: owners,
			},
			Status: corev1.PodStatus{Phase: phase, Reason: reason},
		}
	}

	cfg := &config.K8sConfig{
		DeleteFailedPods:  true,
		DeleteEvictedPods: true,
	}
	s := &Sweeper{}

	tests := []struct {
		name string
		pod  *corev1.Pod
		cfg  *config.K8sConfig
		want bool
	}{
		{
			name: "evicted pod deleted when DeleteEvictedPods enabled",
			pod:  newPod(corev1.PodFailed, "Evicted"),
			cfg:  cfg,
			want: true,
		},
		{
			name: "evicted pod kept when DeleteEvictedPods disabled",
			pod:  newPod(corev1.PodFailed, "Evicted"),
			cfg:  &config.K8sConfig{DeleteFailedPods: true, DeleteEvictedPods: false},
			want: false,
		},
		{
			name: "failed non-job pod deleted when DeleteFailedPods enabled",
			pod:  newPod(corev1.PodFailed, "OOMKilled"),
			cfg:  cfg,
			want: true,
		},
		{
			name: "failed pod kept when DeleteFailedPods disabled",
			pod:  newPod(corev1.PodFailed, "OOMKilled"),
			cfg:  &config.K8sConfig{DeleteFailedPods: false, DeleteEvictedPods: true},
			want: false,
		},
		{
			name: "failed job-owned pod not deleted (jobs handled separately)",
			pod:  newPod(corev1.PodFailed, "Error", "Job"),
			cfg:  cfg,
			want: false,
		},
		{
			name: "running pod never deleted",
			pod:  newPod(corev1.PodRunning, ""),
			cfg:  cfg,
			want: false,
		},
		{
			name: "succeeded pod never deleted",
			pod:  newPod(corev1.PodSucceeded, ""),
			cfg:  cfg,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.shouldDeletePod(tt.pod, tt.cfg); got != tt.want {
				t.Errorf("shouldDeletePod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobConditionTrue(t *testing.T) {
	job := &batchv1.Job{
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
				{Type: batchv1.JobFailed, Status: corev1.ConditionFalse},
			},
		},
	}

	if !jobConditionTrue(job, batchv1.JobComplete) {
		t.Error("expected JobComplete condition to be true")
	}
	if jobConditionTrue(job, batchv1.JobFailed) {
		t.Error("expected JobFailed condition to be false")
	}
	if jobConditionTrue(job, batchv1.JobSuspended) {
		t.Error("expected absent JobSuspended condition to be false")
	}
}

func TestJobFinishTime(t *testing.T) {
	completion := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	failure := time.Date(2026, 8, 2, 13, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		job  *batchv1.Job
		want time.Time
	}{
		{
			name: "successful job uses CompletionTime",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Succeeded:      1,
					CompletionTime: &metav1.Time{Time: completion},
				},
			},
			want: completion,
		},
		{
			name: "failed job uses JobFailed transition time",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Time{Time: failure}},
					},
				},
			},
			want: failure,
		},
		{
			name: "CompletionTime takes precedence over failed condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					CompletionTime: &metav1.Time{Time: completion},
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Time{Time: failure}},
					},
				},
			},
			want: completion,
		},
		{
			name: "running job has zero finish time",
			job:  &batchv1.Job{Status: batchv1.JobStatus{}},
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobFinishTime(tt.job); !got.Equal(tt.want) {
				t.Errorf("jobFinishTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobCleanupRetention(t *testing.T) {
	now := time.Now()
	old := now.AddDate(0, 0, -10)
	recent := now.AddDate(0, 0, -1)

	completedOld := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "old-completed", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &metav1.Time{Time: old},
		},
	}
	completedRecent := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "recent-completed", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Succeeded:      1,
			CompletionTime: &metav1.Time{Time: recent},
		},
	}
	failedOld := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "old-failed", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Time{Time: old}},
			},
		},
	}
	running := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "ns"},
		Status:     batchv1.JobStatus{},
	}

	cfg := &config.K8sConfig{
		DeleteCompletedJobs: true,
		DeleteSucceededJobs: false,
		JobRetentionDays:    7,
	}

	tests := []struct {
		name string
		job  *batchv1.Job
		want bool
	}{
		{"completed job older than retention window is deletable", completedOld, true},
		{"completed job inside retention window is kept", completedRecent, false},
		{"failed job older than retention window is deletable", failedOld, true},
		{"running job is never deletable", running, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldDelete := false
			succeeded := tt.job.Status.CompletionTime != nil && tt.job.Status.Succeeded > 0
			failed := jobConditionTrue(tt.job, batchv1.JobFailed)
			if cfg.DeleteSucceededJobs && succeeded {
				shouldDelete = true
			}
			if cfg.DeleteCompletedJobs && (succeeded || failed) {
				shouldDelete = true
			}
			if !succeeded && !failed {
				shouldDelete = false
			}
			if shouldDelete {
				finishedAt := jobFinishTime(tt.job)
				retentionCutoff := now.AddDate(0, 0, -cfg.JobRetentionDays)
				if !finishedAt.IsZero() && finishedAt.After(retentionCutoff) {
					shouldDelete = false
				}
			}
			if shouldDelete != tt.want {
				t.Errorf("job %q deletable = %v, want %v", tt.job.Name, shouldDelete, tt.want)
			}
		})
	}
}
