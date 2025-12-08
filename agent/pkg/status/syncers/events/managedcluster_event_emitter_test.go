package events

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestCustomizeProvisionJobMessage(t *testing.T) {
	tests := []struct {
		name            string
		reason          string
		originalMessage string
		clusterName     string
		wantMessage     string
	}{
		{
			name:            "SuccessfulCreate reason",
			reason:          "SuccessfulCreate",
			originalMessage: "Created pod: cluster2-0-tncv5-provision-fhd94",
			clusterName:     "cluster2",
			wantMessage:     "Cluster cluster2 provisioning started",
		},
		{
			name:            "Completed reason",
			reason:          "Completed",
			originalMessage: "Job completed",
			clusterName:     "cluster2",
			wantMessage:     "Cluster cluster2 provisioning completed successfully",
		},
		{
			name:            "other reason - generic message",
			reason:          "BackoffLimitExceeded",
			originalMessage: "Job has reached the specified backoff limit",
			clusterName:     "cluster2",
			wantMessage:     "Provisioning cluster2: Job has reached the specified backoff limit",
		},
		{
			name:            "other reason - DeadlineExceeded",
			reason:          "DeadlineExceeded",
			originalMessage: "Job was active longer than specified deadline",
			clusterName:     "prod-cluster",
			wantMessage:     "Provisioning prod-cluster: Job was active longer than specified deadline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := customizeProvisionJobMessage(tt.reason, tt.originalMessage, tt.clusterName)
			if got != tt.wantMessage {
				t.Errorf("customizeProvisionJobMessage() = %q, want %q", got, tt.wantMessage)
			}
		})
	}
}

func TestIsValidProvisionJobEvent(t *testing.T) {
	tests := []struct {
		name      string
		jobName   string
		namespace string
		kind      string
		wantValid bool
	}{
		{
			name:      "valid provision job with single-part hash",
			jobName:   "cluster2-bvpxh-provision",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: true,
		},
		{
			name:      "valid provision job with multi-part hash",
			jobName:   "cluster2-0-bvpxh-provision",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: true,
		},
		{
			name:      "valid provision job with alphanumeric hash",
			jobName:   "prod-abc123-provision",
			namespace: "prod",
			kind:      "Job",
			wantValid: true,
		},
		{
			name:      "valid provision job with multi-part namespace",
			jobName:   "my-cluster-xyz789-provision",
			namespace: "my-cluster",
			kind:      "Job",
			wantValid: true,
		},
		{
			name:      "valid provision job with hyphenated hash",
			jobName:   "cluster2-0-bv-pxh-provision",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: true,
		},
		{
			name:      "invalid - missing provision suffix",
			jobName:   "cluster2-0-bvpxh",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: false,
		},
		{
			name:      "invalid - wrong suffix",
			jobName:   "cluster2-0-bvpxh-deploy",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: false,
		},
		{
			name:      "invalid - no hash (directly provision)",
			jobName:   "cluster2-provision",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: false,
		},
		{
			name:      "invalid - namespace mismatch",
			jobName:   "cluster2-abc-provision",
			namespace: "cluster3",
			kind:      "Job",
			wantValid: false,
		},
		{
			name:      "invalid - empty job name",
			jobName:   "",
			namespace: "cluster2",
			kind:      "Job",
			wantValid: false,
		},
		{
			name:      "invalid - only provision suffix",
			jobName:   "-provision",
			namespace: "",
			kind:      "Job",
			wantValid: false,
		},
		{
			name:      "invalid - not a Job kind",
			jobName:   "cluster2-abc123-provision",
			namespace: "cluster2",
			kind:      "Pod",
			wantValid: false,
		},
		{
			name:      "invalid - Deployment kind",
			jobName:   "cluster2-abc123-provision",
			namespace: "cluster2",
			kind:      "Deployment",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-event",
					Namespace: tt.namespace,
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: tt.kind,
					Name: tt.jobName,
				},
			}
			got := isValidProvisionJobEvent(evt)
			if got != tt.wantValid {
				t.Errorf("isValidProvisionJobEvent(jobName=%q, namespace=%q, kind=%q) = %v, want %v",
					tt.jobName, tt.namespace, tt.kind, got, tt.wantValid)
			}
		})
	}
}

func TestManagedClusterEventPredicate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		event      *corev1.Event
		wantAccept bool
	}{
		{
			name: "accept ManagedCluster event",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1.event1",
					Namespace: "cluster1",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: constants.ManagedClusterKind,
					Name: "cluster1",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: true,
		},
		{
			name: "accept valid provision job event",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job.event1",
					Namespace: "cluster2",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Job",
					Name: "cluster2-abc123-provision",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: true,
		},
		{
			name: "reject provision job with namespace mismatch",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job.event2",
					Namespace: "cluster3",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Job",
					Name: "cluster2-abc123-provision",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: false,
		},
		{
			name: "reject job without provision suffix",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job.event3",
					Namespace: "cluster2",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Job",
					Name: "cluster2-abc123-deploy",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: false,
		},
		{
			name: "reject invalid job name pattern",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job.event4",
					Namespace: "cluster2",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Job",
					Name: "cluster2-provision",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: false,
		},
		{
			name: "reject Pod event",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod.event1",
					Namespace: "default",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
					Name: "test-pod",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: false,
		},
		{
			name: "reject Deployment event",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment.event1",
					Namespace: "default",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Deployment",
					Name: "test-deployment",
				},
				LastTimestamp: metav1.Time{Time: now},
			},
			wantAccept: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := managedClusterEventPredicate(tt.event)
			if got != tt.wantAccept {
				t.Errorf("managedClusterEventPredicate() = %v, want %v", got, tt.wantAccept)
			}
		})
	}
}

func TestGetEventLastTime(t *testing.T) {
	baseTime := time.Date(2025, 12, 7, 10, 0, 0, 0, time.UTC)
	laterTime := baseTime.Add(5 * time.Minute)
	seriesTime := baseTime.Add(10 * time.Minute)

	tests := []struct {
		name     string
		event    *corev1.Event
		wantTime time.Time
	}{
		{
			name: "use CreationTimestamp when no other timestamps",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: baseTime},
				},
			},
			wantTime: baseTime,
		},
		{
			name: "use LastTimestamp when available",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: baseTime},
				},
				LastTimestamp: metav1.Time{Time: laterTime},
			},
			wantTime: laterTime,
		},
		{
			name: "use Series.LastObservedTime when available",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: baseTime},
				},
				LastTimestamp: metav1.Time{Time: laterTime},
				Series: &corev1.EventSeries{
					LastObservedTime: metav1.MicroTime{Time: seriesTime},
				},
			},
			wantTime: seriesTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEventLastTime(tt.event)
			if !got.Time.Equal(tt.wantTime) {
				t.Errorf("getEventLastTime() = %v, want %v", got.Time, tt.wantTime)
			}
		})
	}
}
