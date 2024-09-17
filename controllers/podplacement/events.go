package podplacement

import "github.com/openshift/multiarch-tuning-operator/pkg/utils"

const (
	ArchitecturePredicatesConflict                = "ArchAwarePredicatesConflict"
	ImageArchitectureInspectionError              = "ArchAwareInspectionError"
	ArchitectureAwareNodeAffinitySet              = "ArchAwarePredicateSet"
	ArchitectureAwareGatedPodIgnored              = "ArchAwareGatedPodIgnored"
	ArchitectureAwareSchedulingGateAdded          = "ArchAwareSchedGateAdded"
	ArchitectureAwareSchedulingGateRemovalFailure = "ArchAwareSchedGateRemovalFailed"
	ArchitectureAwareSchedulingGateRemovalSuccess = "ArchAwareSchedGateRemovalSuccess"
	NoSupportedArchitecturesFound                 = "NoSupportedArchitecturesFound"

	SchedulingGateAddedMsg              = "Successfully gated with the " + utils.SchedulingGateName + " scheduling gate"
	SchedulingGateRemovalSuccessMsg     = "Successfully removed the " + utils.SchedulingGateName + " scheduling gate"
	SchedulingGateRemovalFailureMsg     = "Failed to remove the scheduling gate \"" + utils.SchedulingGateName + "\""
	ArchitecturePredicatesConflictMsg   = "All the scheduling predicates already include architecture-specific constraints"
	ArchitecturePredicateSetupMsg       = "Set the nodeAffinity for the architecture to "
	ImageArchitectureInspectionErrorMsg = "Failed to retrieve the supported architectures: "
	NoSupportedArchitecturesFoundMsg    = "Pod cannot be scheduled due to incompatible image architectures; container images have no supported architectures in common"
	ArchitectureAwareGatedPodIgnoredMsg = "The gated pod has been modified and is no longer eligible for architecture-aware scheduling"
)
