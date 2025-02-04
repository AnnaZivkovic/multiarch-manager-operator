package utils

import "k8s.io/apimachinery/pkg/util/sets"

const (
	ControllerNameKey = "controller"
	OperandLabelKey   = "multiarch.openshift.io/operand"
	OperatorName      = "multiarch-tuning-operator"
)

const (
	ArchitectureAmd64   = "amd64"
	ArchitectureArm64   = "arm64"
	ArchitecturePpc64le = "ppc64le"
	ArchitectureS390x   = "s390x"
)

const (
	ArchLabel                       = "kubernetes.io/arch"
	NodeAffinityLabel               = "multiarch.openshift.io/node-affinity"
	PreferredNodeAffinityLabel      = "multiarch.openshift.io/preferred-node-affinity"
	NodeAffinityLabelValueSet       = "set"
	LabelValueNotSet                = "not-set"
	HostnameLabel                   = "kubernetes.io/hostname"
	SchedulingGateLabel             = "multiarch.openshift.io/scheduling-gate"
	SchedulingGateLabelValueGated   = "gated"
	SchedulingGateLabelValueRemoved = "removed"
	PodPlacementFinalizerName       = "finalizers.multiarch.openshift.io/pod-placement"
	SingleArchLabel                 = "multiarch.openshift.io/single-arch"
	MultiArchLabel                  = "multiarch.openshift.io/multi-arch"
	NoSupportedArchLabel            = "multiarch.openshift.io/no-supported-arch"
	ImageInspectionErrorLabel       = "multiarch.openshift.io/image-inspect-error"
	ImageInspectionErrorCountLabel  = "multiarch.openshift.io/image-inspect-error-count"
	LabelGroup                      = "multiarch.openshift.io"
)

const (
	// SchedulingGateName is the name of the Scheduling Gate
	SchedulingGateName            = "multiarch.openshift.io/scheduling-gate"
	MasterNodeSelectorLabel       = "node-role.kubernetes.io/master"
	ControlPlaneNodeSelectorLabel = "node-role.kubernetes.io/control-plane"
)

const (
	PodMutatingWebhookConfigurationName = "pod-placement-mutating-webhook-configuration"
	PodMutatingWebhookName              = "pod-placement-scheduling-gate.multiarch.openshift.io"
	PodPlacementControllerName          = "pod-placement-controller"
	PodPlacementWebhookName             = "pod-placement-web-hook"
)

func AllSupportedArchitecturesSet() sets.Set[string] {
	return sets.New(ArchitectureAmd64, ArchitectureArm64, ArchitecturePpc64le, ArchitectureS390x)
}
