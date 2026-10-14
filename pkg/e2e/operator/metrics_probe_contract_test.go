package operator_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	_ "github.com/openshift/multiarch-tuning-operator/internal/controller/podplacement"
	"github.com/openshift/multiarch-tuning-operator/internal/controller/podplacement/metrics"
)

func TestMetricsProbeFixture(t *testing.T) {
	objects, job := newMetricsProbe("test-probe", "operator-ns")
	if len(objects) != 4 || job.Name != "test-probe" || job.Namespace != metricsProbeNamespace {
		t.Fatal("fixture must create only a uniquely named SA, role, binding, CA and monitoring Job")
	}
	role := objects[1].(*rbacv1.ClusterRole)
	if !reflect.DeepEqual(role.Rules, []rbacv1.PolicyRule{{Verbs: []string{"get"}, NonResourceURLs: []string{"/metrics"}}}) {
		t.Fatalf("unexpected metrics permission: %+v", role.Rules)
	}
	binding := objects[2].(*rbacv1.ClusterRoleBinding)
	if binding.RoleRef.Name != role.Name || binding.RoleRef.Kind != "ClusterRole" || binding.RoleRef.APIGroup != rbacv1.GroupName ||
		!reflect.DeepEqual(binding.Subjects, []rbacv1.Subject{{Kind: "ServiceAccount", Name: objects[0].GetName(), Namespace: metricsProbeNamespace}}) {
		t.Fatal("metrics permission must be bound only to the probe's service account")
	}
	ca := objects[3].(*corev1.ConfigMap)
	if ca.Name != job.Name || ca.Namespace != metricsProbeNamespace || ca.Annotations["service.beta.openshift.io/inject-cabundle"] != "true" {
		t.Fatal("fixture must use a uniquely owned injected service CA")
	}
	podSpec := job.Spec.Template.Spec
	if podSpec.HostNetwork || podSpec.ServiceAccountName != objects[0].GetName() || *podSpec.AutomountServiceAccountToken ||
		podSpec.RestartPolicy != corev1.RestartPolicyNever || *job.Spec.BackoffLimit != 0 || *job.Spec.ActiveDeadlineSeconds != metricsProbeDeadline {
		t.Fatal("probe must use pod networking, bounded execution and only its projected credentials")
	}
	if len(job.Spec.Template.Labels) != 0 || len(podSpec.Containers) != 1 || podSpec.Containers[0].Image != metricsProbeImage {
		t.Fatal("probe must reuse one existing image without workload selector labels")
	}
	container := podSpec.Containers[0]
	if !reflect.DeepEqual(container.Command, []string{"/bin/sh", "-c"}) || !reflect.DeepEqual(container.Args[2:], []string{
		"https://pod-placement-controller.operator-ns.svc:8443/metrics",
		"https://pod-placement-web-hook.operator-ns.svc:8443/metrics",
	}) {
		t.Fatal("probe must request the exact two service DNS endpoints")
	}
	if !*podSpec.SecurityContext.RunAsNonRoot || podSpec.SecurityContext.RunAsUser != nil ||
		podSpec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault ||
		*container.SecurityContext.AllowPrivilegeEscalation || !*container.SecurityContext.ReadOnlyRootFilesystem ||
		!reflect.DeepEqual(container.SecurityContext.Capabilities.Drop, []corev1.Capability{"ALL"}) {
		t.Fatal("probe must allow SCC-assigned UID with restricted security settings")
	}
	projection := podSpec.Volumes[0].Projected
	if len(projection.Sources) != 1 || projection.Sources[0].ServiceAccountToken.Path != "token" ||
		projection.Sources[0].ServiceAccountToken.Audience != "" || *projection.Sources[0].ServiceAccountToken.ExpirationSeconds != 600 {
		t.Fatal("probe must project a short-lived token with the standard API audience")
	}
	if podSpec.Volumes[1].ConfigMap.Name != ca.Name || podSpec.Volumes[1].ConfigMap.Items[0].Key != "service-ca.crt" ||
		podSpec.Volumes[2].EmptyDir == nil || !container.VolumeMounts[0].ReadOnly || !container.VolumeMounts[1].ReadOnly {
		t.Fatal("probe must mount owned service CA, read-only credentials and writable scratch space")
	}
}

func TestMetricsProbeComplete(t *testing.T) {
	for _, test := range []struct {
		name       string
		conditions []batchv1.JobCondition
		complete   bool
		failed     bool
	}{
		{name: "pending"},
		{name: "complete", conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}, complete: true},
		{name: "false complete", conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionFalse}}},
		{name: "deadline", conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded"}}, failed: true},
		{name: "failure wins", conditions: []batchv1.JobCondition{
			{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
		}, failed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			complete, err := metricsProbeComplete(&batchv1.Job{Status: batchv1.JobStatus{Conditions: test.conditions}})
			if complete != test.complete || (err != nil) != test.failed {
				t.Fatalf("completion=%t error=%v", complete, err)
			}
		})
	}
}

func TestMetricsProbeCleanup(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: metricsProbeNamespace, UID: "owned-uid"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "owned-pod", Namespace: metricsProbeNamespace,
		Labels: map[string]string{"batch.kubernetes.io/controller-uid": string(job.UID)},
	}}
	getDeletedJob := func(_ context.Context, key runtimeclient.ObjectKey, _ runtimeclient.Object, _ ...runtimeclient.GetOption) error {
		if key != runtimeclient.ObjectKeyFromObject(job) {
			t.Fatal("cleanup accessed an unrelated object")
		}
		return apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, key.Name)
	}
	t.Run("wait for pods and use UID preconditions", func(t *testing.T) {
		lists := 0
		cl := &metricsProbeCleanupClient{
			get: getDeletedJob,
			delete: func(_ context.Context, object runtimeclient.Object, options ...runtimeclient.DeleteOption) error {
				deleteOptions := &runtimeclient.DeleteOptions{}
				deleteOptions.ApplyOptions(options)
				if object.GetUID() != job.UID || deleteOptions.Preconditions == nil || *deleteOptions.Preconditions.UID != job.UID ||
					deleteOptions.PropagationPolicy == nil || *deleteOptions.PropagationPolicy != metav1.DeletePropagationForeground {
					t.Fatal("cleanup must enforce owned UID and foreground propagation")
				}
				return nil
			},
			list: func(_ context.Context, list runtimeclient.ObjectList, options ...runtimeclient.ListOption) error {
				listOptions := &runtimeclient.ListOptions{}
				listOptions.ApplyOptions(options)
				if listOptions.Namespace != metricsProbeNamespace || listOptions.LabelSelector.String() != "batch.kubernetes.io/controller-uid="+string(job.UID) {
					t.Fatal("cleanup must query only this Job's pods")
				}
				lists++
				if lists == 1 {
					list.(*corev1.PodList).Items = []corev1.Pod{*pod}
				}
				return nil
			},
		}
		if err := deleteMetricsProbeObject(context.Background(), cl, job); err != nil {
			t.Fatal(err)
		}
		if lists != 2 {
			t.Fatal("cleanup returned before asynchronous Job pod deletion")
		}
	})
	t.Run("reject unknown ownership", func(t *testing.T) {
		cl := &metricsProbeCleanupClient{}
		if err := deleteMetricsProbeObject(context.Background(), cl, &corev1.ServiceAccount{}); err == nil {
			t.Fatal("cleanup must reject objects without a created UID")
		}
	})
	t.Run("bounded pod deletion", func(t *testing.T) {
		cl := &metricsProbeCleanupClient{
			get:    getDeletedJob,
			delete: func(context.Context, runtimeclient.Object, ...runtimeclient.DeleteOption) error { return nil },
			list: func(_ context.Context, list runtimeclient.ObjectList, _ ...runtimeclient.ListOption) error {
				list.(*corev1.PodList).Items = []corev1.Pod{*pod}
				return nil
			},
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := deleteMetricsProbeObject(cleanupCtx, cl, job); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected bounded cleanup error, got %v", err)
		}
	})
	t.Run("checked deletion error", func(t *testing.T) {
		denied := errors.New("delete denied")
		cl := &metricsProbeCleanupClient{delete: func(context.Context, runtimeclient.Object, ...runtimeclient.DeleteOption) error { return denied }}
		if err := deleteMetricsProbeObject(context.Background(), cl, job); !errors.Is(err, denied) {
			t.Fatalf("expected checked deletion error, got %v", err)
		}
	})
}

type metricsProbeCleanupClient struct {
	runtimeclient.Client
	get    func(context.Context, runtimeclient.ObjectKey, runtimeclient.Object, ...runtimeclient.GetOption) error
	list   func(context.Context, runtimeclient.ObjectList, ...runtimeclient.ListOption) error
	delete func(context.Context, runtimeclient.Object, ...runtimeclient.DeleteOption) error
}

func (cl *metricsProbeCleanupClient) Get(ctx context.Context, key runtimeclient.ObjectKey, object runtimeclient.Object, options ...runtimeclient.GetOption) error {
	return cl.get(ctx, key, object, options...)
}

func (cl *metricsProbeCleanupClient) List(ctx context.Context, list runtimeclient.ObjectList, options ...runtimeclient.ListOption) error {
	return cl.list(ctx, list, options...)
}

func (cl *metricsProbeCleanupClient) Delete(ctx context.Context, object runtimeclient.Object, options ...runtimeclient.DeleteOption) error {
	return cl.delete(ctx, object, options...)
}

func TestMetricsProbeRegisteredMarker(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, operand := range []string{"controller", "webhook"} {
		t.Run(operand, func(t *testing.T) {
			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(commandCtx, executable, "-test.run=^TestMetricsProbeStartupHandler$", "-test.v")
			command.Env = append(os.Environ(), "MTO_METRICS_PROBE_STARTUP="+operand)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("%s startup handler failed: %v\n%s", operand, err, output)
			}
			t.Logf("%s", output)
		})
	}
}

func TestMetricsProbeStartupHandler(t *testing.T) {
	mode := os.Getenv("MTO_METRICS_PROBE_STARTUP")
	switch mode {
	case "":
		t.Skip("fresh-process helper invoked by TestMetricsProbeRegisteredMarker")
	case "controller", "webhook":
	default:
		t.Fatalf("unknown operand %s", mode)
	}
	if metrics.GatedPodsGauge != nil {
		t.Fatal("operand metrics must not be initialized before the startup request")
	}
	server, err := metricsserver.NewServer(metricsserver.Options{BindAddress: "127.0.0.1:0"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	address, ok := server.(interface{ GetBindAddr() string })
	if !ok {
		t.Fatal("production metrics server must expose its bound address")
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- server.Start(serverCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("metrics server shutdown failed: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("metrics server did not stop")
		}
	})
	httpClient := &http.Client{Timeout: time.Second}
	t.Cleanup(httpClient.CloseIdleConnections)
	var body []byte
	err = wait.PollUntilContextTimeout(serverCtx, 10*time.Millisecond, 3*time.Second, true, func(ctx context.Context) (bool, error) {
		if address.GetBindAddr() == "" {
			return false, nil
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address.GetBindAddr()+"/metrics", nil)
		if err != nil {
			return false, err
		}
		response, err := httpClient.Do(request)
		if err != nil {
			return false, err
		}
		defer func() {
			if err := response.Body.Close(); err != nil {
				t.Errorf("close metrics response: %v", err)
			}
		}()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("startup metrics handler returned HTTP %d", response.StatusCode)
		}
		body, err = io.ReadAll(response.Body)
		return err == nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(metricsProbeMarker) + ` [0-9]+$`).Match(body) {
		t.Fatalf("%s sample missing from startup HTTP handler", metricsProbeMarker)
	}
	if metrics.GatedPodsGauge != nil || regexp.MustCompile(`(?m)^mto_ppo_pods_gated `).Match(body) {
		t.Fatal("startup handler must expose runtime metrics without initializing lazy operand metrics")
	}
	t.Logf("%s startup handler: HTTP 200, %s sample present, previous lazy marker absent", mode, metricsProbeMarker)
}

func TestMetricsProbeScript(t *testing.T) {
	for _, test := range []struct {
		name, status, code, body, diagnostic string
	}{
		{"success", "200", "0", "go_goroutines 12", "verified"},
		{"unauthenticated", "401", "0", "", "authentication/authorization"},
		{"forbidden", "403", "0", "", "authentication/authorization"},
		{"redirect", "302", "0", "", "redirect refused"},
		{"server error", "500", "0", "synthetic-test-token", "unexpected HTTP"},
		{"missing marker", "200", "0", "other_metric 1", "registered metric"},
		{"previous marker", "200", "0", "mto_ppo_pods_gated 0", "registered metric"},
		{"help only", "200", "0", "# HELP go_goroutines help", "registered metric"},
		{"invalid sample", "200", "0", "go_goroutines not-a-number", "registered metric"},
		{"webhook missing marker", "200", "0", "go_goroutines 12", "registered metric"},
		{"controller missing marker", "200", "0", "other_metric 1", "registered metric"},
		{"dns", "000", "6", "", "DNS resolution"},
		{"connection", "000", "7", "", "connection failed"},
		{"timeout", "000", "28", "", "connection/request timeout"},
		{"tls", "000", "60", "", "TLS/CA/hostname verification"},
		{"ca", "000", "77", "", "TLS/CA/hostname verification"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			mockCurl := `#!/bin/sh
set -eu
credentials=$(cat)
printf '%s' "$credentials" >&2
printf 'sensitive-error-must-not-be-logged' >&2
printf '%s\n' "$*" >&2
for argument in "$@"; do endpoint=$argument; done
printf '%s\n' "$endpoint" >>"$MOCK_REQUESTS"
body=$MOCK_BODY
if [ "$endpoint" = 'https://webhook/metrics' ]; then body=$MOCK_WEBHOOK_BODY; fi
for argument in "$@"; do
  if [ "${previous:-}" = '--output' ]; then printf '%s\n%s\n' "$body" 'sensitive-response-must-not-be-logged' >"$argument"; fi
  previous=$argument
done
printf '%s' "$MOCK_STATUS"
exit "$MOCK_CODE"
`
			if err := os.WriteFile(filepath.Join(directory, "curl"), []byte(mockCurl), 0750); err != nil {
				t.Fatal(err)
			}
			tokenFile := filepath.Join(directory, "token")
			if err := os.WriteFile(tokenFile, []byte("synthetic-test-token"), 0600); err != nil {
				t.Fatal(err)
			}
			script := strings.ReplaceAll(metricsProbeScript, "/var/run/secrets/kubernetes.io/serviceaccount/token", tokenFile)
			webhookBody := test.body
			if test.name == "webhook missing marker" {
				webhookBody = "other_metric 1"
			}
			if test.name == "controller missing marker" {
				webhookBody = "go_goroutines 12"
			}
			requestsFile := filepath.Join(directory, "requests")
			commandCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			command := exec.CommandContext(commandCtx, "/bin/sh", "-c", script, "metrics-probe", "https://controller/metrics", "https://webhook/metrics")
			command.Env = append(os.Environ(), "PATH="+directory+":"+os.Getenv("PATH"),
				"MOCK_BODY="+test.body, "MOCK_STATUS="+test.status, "MOCK_CODE="+test.code,
				"MOCK_WEBHOOK_BODY="+webhookBody, "MOCK_REQUESTS="+requestsFile)
			output, err := command.CombinedOutput()
			if (err == nil) != (test.name == "success") || !strings.Contains(string(output), test.diagnostic) {
				t.Fatalf("unexpected outcome: error=%v output=%s", err, output)
			}
			for _, forbidden := range []string{"synthetic-test-token", "sensitive-error", "sensitive-response", "https://", "controller/metrics", "webhook/metrics"} {
				if strings.Contains(string(output), forbidden) {
					t.Fatalf("probe diagnostics leaked %s", forbidden)
				}
			}
			for _, label := range []string{"controller", "webhook"} {
				if !strings.Contains(string(output), label+": ") {
					t.Fatalf("missing safe diagnostic for %s", label)
				}
			}
			requests, err := os.ReadFile(requestsFile)
			if err != nil {
				t.Fatal(err)
			}
			if string(requests) != "https://controller/metrics\nhttps://webhook/metrics\n" {
				t.Fatal("both endpoints must be requested exactly once, including after a failure")
			}
			if test.name == "controller missing marker" &&
				!strings.Contains(string(output), "webhook: HTTP 200 and registered metric go_goroutines verified") {
				t.Fatal("controller failure must not skip independent webhook verification")
			}
			if test.name == "webhook missing marker" &&
				!strings.Contains(string(output), "controller: HTTP 200 and registered metric go_goroutines verified") {
				t.Fatal("webhook failure must not mask independent controller verification")
			}
			if test.name == "success" && strings.Count(string(output), "verified") != 2 {
				t.Fatal("both endpoints must be independently verified")
			}
		})
	}
	for _, option := range []string{"--disable --config -", "--noproxy '*'", "--proto '=https'", "--max-redirs 0", "--connect-timeout 5", "--max-time 20", "--cacert"} {
		if !strings.Contains(metricsProbeScript, option) {
			t.Errorf("missing secure request option %s", option)
		}
	}
	for _, forbidden := range []string{"--insecure", "--location", "--verbose", "set -x"} {
		if strings.Contains(metricsProbeScript, forbidden) {
			t.Errorf("unsafe request option %s", forbidden)
		}
	}
	if !strings.Contains(metricsProbeScript, "'^"+metricsProbeMarker+" [0-9]+$'") || metricsProbeImage != "quay.io/openshifttest/hello-openshift:1.2.0" {
		t.Fatal("script marker and existing image must match verified contracts")
	}
}
