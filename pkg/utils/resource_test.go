package utils

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/operator/events"
)

func TestServiceSpecMatchesRequired(t *testing.T) {
	basePorts := []corev1.ServicePort{
		{
			Name:       "https",
			Port:       443,
			TargetPort: intstr.FromInt32(9443),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "metrics",
			Port:       8443,
			TargetPort: intstr.FromInt32(8443),
			Protocol:   corev1.ProtocolTCP,
		},
	}
	baseSelector := map[string]string{
		"app":        "test",
		"controller": "pod-placement-controller",
	}

	tests := []struct {
		name     string
		existing corev1.ServiceSpec
		required corev1.ServiceSpec
		want     bool
	}{
		{
			name: "identical specs match",
			existing: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			want: true,
		},
		{
			name: "empty required type matches ClusterIP",
			existing: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
			},
			want: true,
		},
		{
			name: "server-defaulted fields ignored",
			existing: corev1.ServiceSpec{
				Ports:           basePorts,
				Selector:        baseSelector,
				Type:            corev1.ServiceTypeClusterIP,
				ClusterIP:       "10.0.0.1",
				ClusterIPs:      []string{"10.0.0.1"},
				SessionAffinity: corev1.ServiceAffinityNone,
				IPFamilies:      []corev1.IPFamily{corev1.IPv4Protocol},
			},
			required: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
			},
			want: true,
		},
		{
			name: "changed port number detected",
			existing: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 8080, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
			},
			want: false,
		},
		{
			name: "changed port name detected",
			existing: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
			},
			want: false,
		},
		{
			name: "changed targetPort detected",
			existing: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
			},
			want: false,
		},
		{
			name: "changed protocol detected",
			existing: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolUDP},
				},
				Selector: baseSelector,
			},
			want: false,
		},
		{
			name: "different number of ports detected",
			existing: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
			},
			want: false,
		},
		{
			name: "changed selector detected",
			existing: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: basePorts,
				Selector: map[string]string{
					"app": "different",
				},
			},
			want: false,
		},
		{
			name: "changed type detected",
			existing: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports:    basePorts,
				Selector: baseSelector,
				Type:     corev1.ServiceTypeNodePort,
			},
			want: false,
		},
		{
			name: "nil selector in both matches",
			existing: corev1.ServiceSpec{
				Ports: basePorts,
				Type:  corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: basePorts,
			},
			want: true,
		},
		{
			name: "empty protocol defaults to TCP",
			existing: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443)},
				},
				Selector: baseSelector,
			},
			want: true,
		},
		{
			name: "nodePort in existing ignored",
			existing: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP, NodePort: 30443},
				},
				Selector: baseSelector,
				Type:     corev1.ServiceTypeClusterIP,
			},
			required: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
				},
				Selector: baseSelector,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceSpecMatchesRequired(tt.existing, tt.required)
			if got != tt.want {
				t.Errorf("serviceSpecMatchesRequired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newTestRecorder() events.Recorder {
	return events.NewInMemoryRecorder("test", clock.RealClock{})
}

func TestApplyDeployment_SkipsNoOpUpdate(t *testing.T) {
	ResetGenerations()
	defer ResetGenerations()

	required := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}

	client := kubefake.NewSimpleClientset().AppsV1()
	recorder := newTestRecorder()

	result1, modified1, err := applyDeployment(context.Background(), client, recorder, required)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if !modified1 {
		t.Fatal("first apply should report modified=true (creation)")
	}

	// Second apply with identical spec should skip
	result2, modified2, err := applyDeployment(context.Background(), client, recorder, required)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if modified2 {
		t.Fatal("second apply should report modified=false (no-op)")
	}
	if result2.Generation != result1.Generation {
		t.Fatalf("generation should be stable: got %d, want %d", result2.Generation, result1.Generation)
	}
}

func TestApplyDeployment_DetectsExternalChange(t *testing.T) {
	ResetGenerations()
	defer ResetGenerations()

	required := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}

	client := kubefake.NewSimpleClientset().AppsV1()
	recorder := newTestRecorder()

	// First apply (creation)
	_, _, err := applyDeployment(context.Background(), client, recorder, required)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Simulate external modification. A real API server bumps generation on
	// spec changes; the fake clientset does not, so we bump it manually.
	existing, err := client.Deployments("test-ns").Get(context.Background(), "test-deploy", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	existing.Spec.Replicas = int32Ptr(5)
	existing.Generation++
	_, err = client.Deployments("test-ns").Update(context.Background(), existing, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("external update: %v", err)
	}

	// Next apply should detect the external change and re-apply
	result, modified, err := applyDeployment(context.Background(), client, recorder, required)
	if err != nil {
		t.Fatalf("apply after external change: %v", err)
	}
	if !modified {
		t.Fatal("apply after external change should report modified=true")
	}
	if *result.Spec.Replicas != 2 {
		t.Fatalf("replicas should be restored to 2, got %d", *result.Spec.Replicas)
	}
}

func TestApplyDaemonSet_SkipsNoOpUpdate(t *testing.T) {
	ResetGenerations()
	defer ResetGenerations()

	required := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "test-ns",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}},
			},
		},
	}

	client := kubefake.NewSimpleClientset().AppsV1()
	recorder := newTestRecorder()

	_, modified1, err := applyDaemonSet(context.Background(), client, recorder, required)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if !modified1 {
		t.Fatal("first apply should report modified=true (creation)")
	}

	_, modified2, err := applyDaemonSet(context.Background(), client, recorder, required)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if modified2 {
		t.Fatal("second apply should report modified=false (no-op)")
	}
}

func TestApplyService_SkipsNoOpUpdate(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
			},
			Selector: map[string]string{"app": "test"},
		},
	}

	fakeClient := kubefake.NewSimpleClientset()
	recorder := newTestRecorder()

	// First apply (creation)
	_, modified1, err := applyService(context.Background(), fakeClient.CoreV1(), recorder, svc)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if !modified1 {
		t.Fatal("first apply should report modified=true (creation)")
	}

	// Second apply with identical spec should skip
	_, modified2, err := applyService(context.Background(), fakeClient.CoreV1(), recorder, svc)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if modified2 {
		t.Fatal("second apply should report modified=false (no-op)")
	}
}

func TestApplyService_DetectsPortChange(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(9443), Protocol: corev1.ProtocolTCP},
			},
			Selector: map[string]string{"app": "test"},
		},
	}

	fakeClient := kubefake.NewSimpleClientset()
	recorder := newTestRecorder()

	// Create
	_, _, err := applyService(context.Background(), fakeClient.CoreV1(), recorder, svc)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Simulate external port change
	existing, _ := fakeClient.CoreV1().Services("test-ns").Get(context.Background(), "test-svc", metav1.GetOptions{})
	existing.Spec.Ports[0].Port = 8080
	_, err = fakeClient.CoreV1().Services("test-ns").Update(context.Background(), existing, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("external update: %v", err)
	}

	// Apply should detect the change and update
	result, modified, err := applyService(context.Background(), fakeClient.CoreV1(), recorder, svc)
	if err != nil {
		t.Fatalf("apply after change: %v", err)
	}
	if !modified {
		t.Fatal("should report modified=true after external port change")
	}
	if result.Spec.Ports[0].Port != 443 {
		t.Fatalf("port should be restored to 443, got %d", result.Spec.Ports[0].Port)
	}
}

func TestGenerations_Isolation(t *testing.T) {
	ResetGenerations()
	defer ResetGenerations()

	if GenerationsLen() != 0 {
		t.Fatalf("generations should be empty after reset, got %d entries", GenerationsLen())
	}

	required := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "b"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "i"}}},
			},
		},
	}
	client := kubefake.NewSimpleClientset().AppsV1()
	recorder := newTestRecorder()
	if _, _, err := applyDeployment(context.Background(), client, recorder, required); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if GenerationsLen() == 0 {
		t.Fatal("generations should have entries after apply")
	}

	ResetGenerations()
	if GenerationsLen() != 0 {
		t.Fatal("reset should clear all entries")
	}
}

func int32Ptr(i int32) *int32 { return &i }
