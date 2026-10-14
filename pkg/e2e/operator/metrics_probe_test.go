package operator_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/multiarch-tuning-operator/pkg/e2e"
	"github.com/openshift/multiarch-tuning-operator/pkg/testing/builder"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

const (
	metricsProbeNamespace = "openshift-monitoring"
	metricsProbeImage     = e2e.HelloopenshiftPublicMultiarchImage
	metricsProbeMarker    = "go_goroutines"
	metricsProbeDeadline  = int64(180)
	metricsProbeScript    = `set -eu
response=$(mktemp)
trap 'rm -f "$response"' EXIT
failed=0
endpoint_number=0
for endpoint in "$@"; do
  endpoint_number=$((endpoint_number + 1))
  case "$endpoint_number" in
    1) label=controller ;;
    2) label=webhook ;;
    *) exit 1 ;;
  esac
  if status=$(printf 'header = "Authorization: Bearer %s"\n' "$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" |
    curl --disable --config - --silent --noproxy '*' --proto '=https' --max-redirs 0 \
      --connect-timeout 5 --max-time 20 --cacert /var/run/service-ca/service-ca.crt \
      --output "$response" --write-out '%{http_code}' "$endpoint" 2>/dev/null); then
    case "$status" in
      200) ;;
      401|403) printf '%s: authentication/authorization failed (HTTP %s)\n' "$label" "$status"; failed=1; continue ;;
      3??) printf '%s: redirect refused (HTTP %s)\n' "$label" "$status"; failed=1; continue ;;
      *) printf '%s: unexpected HTTP %s\n' "$label" "$status"; failed=1; continue ;;
    esac
  else
    code=$?
    case "$code" in
      5|6) failure='DNS resolution' ;;
      7) failure='connection' ;;
      28) failure='connection/request timeout' ;;
      35|51|58|60|77) failure='TLS/CA/hostname verification' ;;
      *) failure='HTTPS request' ;;
    esac
    printf '%s: %s failed (curl exit %s)\n' "$label" "$failure" "$code"
    failed=1
    continue
  fi
  if ! grep -Eq '^go_goroutines [0-9]+$' "$response"; then
    printf '%s: registered metric go_goroutines sample missing\n' "$label"
    failed=1
    continue
  fi
  printf '%s: HTTP 200 and registered metric go_goroutines verified\n' "$label"
done
exit "$failed"
`
)

func newMetricsProbe(name, operatorNamespace string) ([]runtimeclient.Object, *batchv1.Job) {
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metricsProbeNamespace}}
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, NonResourceURLs: []string{"/metrics"}}},
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: metricsProbeNamespace}},
	}
	ca := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: metricsProbeNamespace,
		Annotations: map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
	}}
	podSpec := builder.NewPodSpec().WithContainersImages(metricsProbeImage).
		WithCommand("/bin/sh", "-c").
		WithArgs(metricsProbeScript, "metrics-probe",
			fmt.Sprintf("https://%s.%s.svc:8443/metrics", utils.PodPlacementControllerName, operatorNamespace),
			fmt.Sprintf("https://%s.%s.svc:8443/metrics", utils.PodPlacementWebhookName, operatorNamespace)).
		WithRestartPolicy(corev1.RestartPolicyNever).Build()
	podSpec.ServiceAccountName = name
	podSpec.AutomountServiceAccountToken = utils.NewPtr(false)
	podSpec.HostNetwork = false
	podSpec.SecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot: utils.NewPtr(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	podSpec.Containers[0].Name = "metrics-probe"
	podSpec.Containers[0].SecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: utils.NewPtr(false),
		ReadOnlyRootFilesystem:   utils.NewPtr(true),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
	podSpec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{Name: "token", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true},
		{Name: "service-ca", MountPath: "/var/run/service-ca", ReadOnly: true},
		{Name: "tmp", MountPath: "/tmp"},
	}
	podSpec.Volumes = []corev1.Volume{
		{Name: "token", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
			DefaultMode: utils.NewPtr(int32(0444)),
			Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
				Path: "token", ExpirationSeconds: utils.NewPtr(int64(600)),
			}}},
		}}},
		{Name: "service-ca", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: name},
			Items:                []corev1.KeyToPath{{Key: "service-ca.crt", Path: "service-ca.crt"}},
		}}},
		{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	job := builder.NewJob().WithName(name).WithNamespace(metricsProbeNamespace).WithPodSpec(podSpec).Build()
	job.Spec.BackoffLimit = utils.NewPtr(int32(0))
	job.Spec.ActiveDeadlineSeconds = utils.NewPtr(metricsProbeDeadline)
	return []runtimeclient.Object{serviceAccount, role, binding, ca}, job
}

func verifyDefaultDenyMetrics(parent context.Context, cl runtimeclient.Client, operatorNamespace string) {
	probeCtx, cancel := context.WithTimeout(parent, 8*time.Minute)
	defer cancel()
	for _, name := range []string{utils.PodPlacementControllerName, utils.PodPlacementWebhookName} {
		Eventually(probeCtx, func(g Gomega) {
			deployment := &appsv1.Deployment{}
			g.Expect(metricsProbeErrorCategory(cl.Get(probeCtx, runtimeclient.ObjectKey{Name: name, Namespace: operatorNamespace}, deployment))).To(Equal("None"))
			g.Expect(deployment.Status.ObservedGeneration).To(BeNumerically(">=", deployment.Generation))
			g.Expect(deployment.Status.ReadyReplicas).To(BeNumerically(">", 0))
			g.Expect(deployment.Status.UnavailableReplicas).To(BeZero())
		}, e2e.WaitShort).Should(Succeed(), "metrics operand %s must be ready", name)
	}
	objects, job := newMetricsProbe("mto-metrics-"+string(uuid.NewUUID()), operatorNamespace)
	jobDeleted := false
	for _, object := range objects {
		Expect(metricsProbeErrorCategory(cl.Create(probeCtx, object))).To(Equal("None"), "create test-owned %T", object)
		DeferCleanup(func() {
			Expect(job.UID == "" || jobDeleted).To(BeTrue(),
				"retaining test-owned %T because Job pod deletion did not finish", object)
			Expect(metricsProbeErrorCategory(deleteMetricsProbeObject(context.Background(), cl, object))).To(Equal("None"), "delete test-owned %T", object)
		})
	}
	ca := objects[3].(*corev1.ConfigMap)
	Eventually(probeCtx, func(g Gomega) {
		g.Expect(metricsProbeErrorCategory(cl.Get(probeCtx, runtimeclient.ObjectKeyFromObject(ca), ca))).To(Equal("None"))
		g.Expect(ca.Data["service-ca.crt"]).NotTo(BeEmpty(), "service CA injection must finish before the Job is created")
	}, time.Minute).Should(Succeed())
	Expect(metricsProbeErrorCategory(cl.Create(probeCtx, job))).To(Equal("None"), "create metrics probe Job (check monitoring namespace admission on failure)")
	DeferCleanup(func() {
		failed := CurrentSpecReport().Failed()
		if failed {
			metricsProbeDiagnostics(cl, job)
		}
		err := deleteMetricsProbeObject(context.Background(), cl, job)
		if err != nil && !failed {
			metricsProbeDiagnostics(cl, job)
		}
		jobDeleted = err == nil
		Expect(metricsProbeErrorCategory(err)).To(Equal("None"), "delete metrics probe Job")
	})
	Eventually(probeCtx, func() (bool, error) {
		current := &batchv1.Job{}
		if err := cl.Get(probeCtx, runtimeclient.ObjectKeyFromObject(job), current); err != nil {
			return false, fmt.Errorf("get metrics probe Job: %s", metricsProbeErrorCategory(err))
		}
		complete, err := metricsProbeComplete(current)
		if err != nil {
			StopTrying(err.Error()).Now()
		}
		return complete, err
	}, 4*time.Minute).Should(BeTrue(), "metrics probe must complete successfully; diagnostics report only allowlisted status and failure categories")
}

func metricsProbeComplete(job *batchv1.Job) (bool, error) {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return false, fmt.Errorf("metrics probe Job failed: %s", metricsProbeReasonCategory(condition.Reason))
		}
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return true, nil
		}
	}
	return false, nil
}

func deleteMetricsProbeObject(parent context.Context, cl runtimeclient.Client, object runtimeclient.Object) error {
	cleanupCtx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	uid := object.GetUID()
	if uid == "" {
		return fmt.Errorf("refusing to delete %T %s without its created UID", object, object.GetName())
	}
	if err := runtimeclient.IgnoreNotFound(cl.Delete(cleanupCtx, object, &runtimeclient.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &uid}, PropagationPolicy: utils.NewPtr(metav1.DeletePropagationForeground),
	})); err != nil {
		return fmt.Errorf("delete test-owned %T %s: %w", object, object.GetName(), err)
	}
	err := wait.PollUntilContextCancel(cleanupCtx, time.Second, true, func(context.Context) (bool, error) {
		current := object.DeepCopyObject().(runtimeclient.Object)
		if err := cl.Get(cleanupCtx, runtimeclient.ObjectKeyFromObject(object), current); !errors.IsNotFound(err) {
			return false, err
		}
		if job, ok := object.(*batchv1.Job); ok {
			pods := &corev1.PodList{}
			if err := cl.List(cleanupCtx, pods, runtimeclient.InNamespace(job.Namespace),
				runtimeclient.MatchingLabels{"batch.kubernetes.io/controller-uid": string(uid)}); err != nil {
				return false, err
			}
			return len(pods.Items) == 0, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for test-owned %T %s and any Job pods to be deleted: %w", object, object.GetName(), err)
	}
	return nil
}

func metricsProbeDiagnostics(cl runtimeclient.Client, job *batchv1.Job) {
	diagnosticCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	writeMetricsProbeDiagnostics(diagnosticCtx, GinkgoWriter, cl, job)
}

// Never emit object names, free-form API messages, or container logs. Even
// status/reason fields must match fixed allowlists before reaching CI output.
// Diagnostic writes are best-effort so output errors do not mask the original test failure.
func writeMetricsProbeDiagnostics(diagnosticCtx context.Context, output io.Writer, cl runtimeclient.Client, job *batchv1.Job) {
	current := &batchv1.Job{}
	if err := cl.Get(diagnosticCtx, runtimeclient.ObjectKeyFromObject(job), current); err != nil {
		_, _ = fmt.Fprintf(output, "metrics probe Job status unavailable: category=%s\n", metricsProbeErrorCategory(err))
	} else {
		_, _ = fmt.Fprintf(output, "metrics probe Job active=%d succeeded=%d failed=%d\n",
			current.Status.Active, current.Status.Succeeded, current.Status.Failed)
		for _, condition := range current.Status.Conditions {
			_, _ = fmt.Fprintf(output, "metrics probe Job condition type=%s status=%s reason=%s\n",
				metricsProbeAllowedValue(string(condition.Type), "Complete", "Failed", "FailureTarget", "SuccessCriteriaMet", "Suspended"),
				metricsProbeConditionStatus(condition.Status), metricsProbeReasonCategory(condition.Reason))
		}
	}
	pods := &corev1.PodList{}
	if err := cl.List(diagnosticCtx, pods, runtimeclient.InNamespace(job.Namespace),
		runtimeclient.MatchingLabels{"batch.kubernetes.io/controller-uid": string(job.UID)}); err != nil {
		_, _ = fmt.Fprintf(output, "metrics probe pods unavailable: category=%s\n", metricsProbeErrorCategory(err))
		return
	}
	uids := map[string]bool{string(job.UID): true}
	for _, pod := range pods.Items {
		uids[string(pod.UID)] = true
		_, _ = fmt.Fprintf(output, "metrics probe pod phase=%s\n",
			metricsProbeAllowedValue(string(pod.Status.Phase), "Pending", "Running", "Succeeded", "Failed", "Unknown"))
		for _, condition := range pod.Status.Conditions {
			_, _ = fmt.Fprintf(output, "metrics probe pod condition type=%s status=%s reason=%s\n",
				metricsProbeAllowedValue(string(condition.Type), "PodScheduled", "PodReadyToStartContainers", "Initialized", "Ready", "ContainersReady", "DisruptionTarget"),
				metricsProbeConditionStatus(condition.Status), metricsProbeReasonCategory(condition.Reason))
		}
		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if status.State.Waiting != nil {
				_, _ = fmt.Fprintf(output, "metrics probe container state=Waiting reason=%s\n",
					metricsProbeReasonCategory(status.State.Waiting.Reason))
			}
			if status.State.Terminated != nil {
				_, _ = fmt.Fprintf(output, "metrics probe container state=Terminated reason=%s exit=%d\n",
					metricsProbeReasonCategory(status.State.Terminated.Reason), status.State.Terminated.ExitCode)
			}
		}
	}
	for uid := range uids {
		events := &corev1.EventList{}
		if err := cl.List(diagnosticCtx, events, runtimeclient.InNamespace(job.Namespace),
			runtimeclient.MatchingFields{"involvedObject.uid": uid}); err != nil {
			_, _ = fmt.Fprintf(output, "metrics probe events unavailable: category=%s\n", metricsProbeErrorCategory(err))
			continue
		}
		for _, event := range events.Items {
			_, _ = fmt.Fprintf(output, "metrics probe event type=%s reason=%s\n",
				metricsProbeAllowedValue(event.Type, corev1.EventTypeNormal, corev1.EventTypeWarning),
				metricsProbeReasonCategory(event.Reason))
		}
	}
}

func metricsProbeAllowedValue(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return candidate
		}
	}
	return "Other"
}

func metricsProbeConditionStatus(status corev1.ConditionStatus) string {
	return metricsProbeAllowedValue(string(status), "True", "False", "Unknown")
}

func metricsProbeReasonCategory(reason string) string {
	return metricsProbeAllowedValue(reason,
		"DeadlineExceeded", "BackoffLimitExceeded", "FailedCreate", "FailedScheduling",
		"FailedMount", "FailedAttachVolume", "Failed", "ErrImagePull", "ImagePullBackOff",
		"InvalidImageName", "CreateContainerConfigError", "CreateContainerError",
		"CrashLoopBackOff", "RunContainerError", "ContainerCannotRun", "OOMKilled",
		"Error", "Completed", "Evicted", "Unschedulable", "ContainersNotReady",
		"PodInitializing", "Scheduled", "Pulling", "Pulled", "Created", "Started",
		"Killing", "BackOff", "SuccessfulCreate")
}

func metricsProbeErrorCategory(err error) string {
	switch {
	case err == nil:
		return "None"
	case stderrors.Is(err, context.DeadlineExceeded), errors.IsTimeout(err), errors.IsServerTimeout(err):
		return "Timeout"
	case stderrors.Is(err, context.Canceled):
		return "Canceled"
	case errors.IsForbidden(err):
		return "Forbidden"
	case errors.IsUnauthorized(err):
		return "Unauthorized"
	case errors.IsNotFound(err):
		return "NotFound"
	case errors.IsAlreadyExists(err):
		return "AlreadyExists"
	case errors.IsConflict(err):
		return "Conflict"
	default:
		return "Other"
	}
}
