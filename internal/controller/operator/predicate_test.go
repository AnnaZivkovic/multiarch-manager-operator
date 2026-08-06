package operator

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	multiarchv1beta1 "github.com/openshift/multiarch-tuning-operator/api/v1beta1"
)

func TestCppcUpdatePredicate(t *testing.T) {
	now := metav1.Now()
	p := cppcUpdatePredicate()

	tests := []struct {
		name string
		old  *multiarchv1beta1.ClusterPodPlacementConfig
		new  *multiarchv1beta1.ClusterPodPlacementConfig
		want bool
	}{
		{
			name: "generation change passes",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
			},
			want: true,
		},
		{
			name: "deletionTimestamp change passes",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1, DeletionTimestamp: &now},
			},
			want: true,
		},
		{
			name: "finalizer change passes",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1, Finalizers: []string{"cleanup"}},
			},
			want: true,
		},
		{
			name: "status-only update filtered",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: multiarchv1beta1.ClusterPodPlacementConfigStatus{
					Conditions: []metav1.Condition{
						{Type: "Available", Status: metav1.ConditionTrue, LastTransitionTime: now, Reason: "Ready"},
					},
				},
			},
			want: false,
		},
		{
			name: "label-only update filtered",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1, Labels: map[string]string{"foo": "bar"}},
			},
			want: false,
		},
		{
			name: "annotation-only update filtered",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1, Annotations: map[string]string{"note": "test"}},
			},
			want: false,
		},
		{
			name: "no change filtered",
			old: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1, Finalizers: []string{"cleanup"}},
			},
			new: &multiarchv1beta1.ClusterPodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Generation: 1, Finalizers: []string{"cleanup"}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Update(event.UpdateEvent{
				ObjectOld: tt.old,
				ObjectNew: tt.new,
			})
			if got != tt.want {
				t.Errorf("UpdateFunc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCppcUpdatePredicate_CreateDeleteGenericPassThrough(t *testing.T) {
	p := cppcUpdatePredicate()
	now := metav1.Now()
	obj := &multiarchv1beta1.ClusterPodPlacementConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", DeletionTimestamp: &now},
	}

	if !p.Create(event.CreateEvent{Object: obj}) {
		t.Error("CreateFunc should always return true")
	}
	if !p.Delete(event.DeleteEvent{Object: obj}) {
		t.Error("DeleteFunc should always return true")
	}
	if !p.Generic(event.GenericEvent{Object: obj}) {
		t.Error("GenericFunc should always return true")
	}
}

func TestCppcUpdatePredicate_BothTimestampsSet(t *testing.T) {
	p := cppcUpdatePredicate()
	t1 := metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	t2 := metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC))

	got := p.Update(event.UpdateEvent{
		ObjectOld: &multiarchv1beta1.ClusterPodPlacementConfig{
			ObjectMeta: metav1.ObjectMeta{Generation: 1, DeletionTimestamp: &t1},
		},
		ObjectNew: &multiarchv1beta1.ClusterPodPlacementConfig{
			ObjectMeta: metav1.ObjectMeta{Generation: 1, DeletionTimestamp: &t2},
		},
	})
	if !got {
		t.Error("different deletionTimestamps should pass")
	}
}
