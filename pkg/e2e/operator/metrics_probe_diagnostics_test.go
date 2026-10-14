package operator_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMetricsProbeDiagnosticsAllowlisted(t *testing.T) {
	const sensitive = "https://internal.example.invalid/metrics?token=synthetic-secret"
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: sensitive, Namespace: metricsProbeNamespace, UID: "job-uid",
	}}
	cl := &metricsProbeCleanupClient{
		get: func(_ context.Context, _ runtimeclient.ObjectKey, object runtimeclient.Object, _ ...runtimeclient.GetOption) error {
			object.(*batchv1.Job).Status = batchv1.JobStatus{
				Active: 1, Succeeded: 2, Failed: 3,
				Conditions: []batchv1.JobCondition{
					{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded", Message: sensitive},
					{Type: batchv1.JobConditionType(sensitive), Status: corev1.ConditionStatus(sensitive), Reason: sensitive, Message: sensitive},
				},
			}
			return nil
		},
		list: func(_ context.Context, list runtimeclient.ObjectList, options ...runtimeclient.ListOption) error {
			listOptions := &runtimeclient.ListOptions{}
			listOptions.ApplyOptions(options)
			if listOptions.Namespace != metricsProbeNamespace {
				t.Fatal("diagnostics queried an unrelated namespace")
			}
			switch list := list.(type) {
			case *corev1.PodList:
				if listOptions.LabelSelector.String() != "batch.kubernetes.io/controller-uid=job-uid" {
					t.Fatal("diagnostics queried unrelated pods")
				}
				list.Items = []corev1.Pod{{
					ObjectMeta: metav1.ObjectMeta{Name: sensitive, UID: "pod-uid"},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending, Message: sensitive, Reason: sensitive, PodIP: sensitive,
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: sensitive},
							{Type: corev1.PodConditionType(sensitive), Status: corev1.ConditionStatus(sensitive), Reason: sensitive, Message: sensitive},
						},
						InitContainerStatuses: []corev1.ContainerStatus{{Name: sensitive, State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: sensitive},
						}}},
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: sensitive, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: sensitive, Message: sensitive}}},
							{Name: sensitive, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
								Reason: "Error", ExitCode: 7, Message: sensitive, ContainerID: sensitive,
							}}},
							{Name: sensitive, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
								Reason: sensitive, ExitCode: 1, Message: sensitive,
							}}},
						},
					},
				}, {ObjectMeta: metav1.ObjectMeta{UID: "other-pod-uid"}, Status: corev1.PodStatus{Phase: corev1.PodPhase(sensitive)}}}
			case *corev1.EventList:
				selector := listOptions.FieldSelector.String()
				if selector != "involvedObject.uid=job-uid" && selector != "involvedObject.uid=pod-uid" && selector != "involvedObject.uid=other-pod-uid" {
					t.Fatal("diagnostics queried unrelated events")
				}
				list.Items = []corev1.Event{
					{Type: corev1.EventTypeWarning, Reason: "FailedMount", Message: sensitive,
						InvolvedObject: corev1.ObjectReference{Name: sensitive, Kind: sensitive}},
					{Type: sensitive, Reason: sensitive, Message: sensitive},
				}
			default:
				t.Fatalf("diagnostics accessed unexpected resource %T", list)
			}
			return nil
		},
	}
	var output bytes.Buffer
	writeMetricsProbeDiagnostics(context.Background(), &output, cl, job)
	text := output.String()
	for _, forbidden := range []string{sensitive, "internal.example.invalid", "synthetic-secret", "https://", "job-uid", "pod-uid"} {
		if strings.Contains(text, forbidden) {
			t.Fatal("diagnostics disclosed a sensitive or identifying field")
		}
	}
	for _, expected := range []string{
		"Job active=1 succeeded=2 failed=3",
		"Job condition type=Failed status=True reason=DeadlineExceeded",
		"Job condition type=Other status=Other reason=Other",
		"pod phase=Pending", "pod phase=Other",
		"pod condition type=PodScheduled status=False reason=Unschedulable",
		"pod condition type=Other status=Other reason=Other",
		"container state=Waiting reason=ImagePullBackOff", "container state=Waiting reason=Other",
		"container state=Terminated reason=Error exit=7", "container state=Terminated reason=Other exit=1",
		"event type=Warning reason=FailedMount", "event type=Other reason=Other",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("missing allowlisted diagnostic %q", expected)
		}
	}
}

func TestMetricsProbeDiagnosticsAPIErrorRedaction(t *testing.T) {
	const sensitive = "server returned https://internal.example.invalid synthetic-secret"
	for _, failure := range []string{"job", "pods", "events"} {
		t.Run(failure, func(t *testing.T) {
			cl := &metricsProbeCleanupClient{
				get: func(context.Context, runtimeclient.ObjectKey, runtimeclient.Object, ...runtimeclient.GetOption) error {
					if failure == "job" {
						return apierrors.NewForbidden(schema.GroupResource{Resource: "jobs"}, sensitive, errors.New(sensitive))
					}
					return nil
				},
				list: func(_ context.Context, list runtimeclient.ObjectList, _ ...runtimeclient.ListOption) error {
					if _, ok := list.(*corev1.PodList); ok && failure == "pods" {
						return errors.New(sensitive)
					}
					if _, ok := list.(*corev1.EventList); ok && failure == "events" {
						return fmt.Errorf("%s: %w", sensitive, context.DeadlineExceeded)
					}
					return nil
				},
			}
			var output bytes.Buffer
			writeMetricsProbeDiagnostics(context.Background(), &output, cl, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: sensitive, Namespace: metricsProbeNamespace, UID: types.UID(sensitive)},
			})
			text := output.String()
			if strings.Contains(text, sensitive) || strings.Contains(text, "internal.example.invalid") || strings.Contains(text, "synthetic-secret") {
				t.Fatal("diagnostics disclosed an API error message")
			}
			expected := map[string]string{"job": "Job status unavailable: category=Forbidden", "pods": "pods unavailable: category=Other", "events": "events unavailable: category=Timeout"}
			if !strings.Contains(text, expected[failure]) {
				t.Errorf("missing sanitized error category %q", expected[failure])
			}
		})
	}
}

func TestMetricsProbeCompleteRedactsUnknownReason(t *testing.T) {
	complete, err := metricsProbeComplete(&batchv1.Job{Status: batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			Reason: "https://internal.example.invalid synthetic-secret", Message: "synthetic-secret"}},
	}})
	if complete || err == nil || err.Error() != "metrics probe Job failed: Other" {
		t.Fatal("Job failure must expose only an allowlisted reason category")
	}
}
