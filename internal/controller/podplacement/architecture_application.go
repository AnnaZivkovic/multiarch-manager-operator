/*
Copyright 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podplacement

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

// applyArchitectureNodeAffinity applies new architecture constraints to a pod's node affinity
// by updating in-place the requiredDuringSchedulingIgnoredDuringExecution matchExpressions for
// kubernetes.io/arch using the In operator.
// It initializes the affinity structure if needed and merges the architecture constraint
// into existing terms to avoid Kubernetes rejecting updates due to NodeSelectorTerms modifications.
// The architectures list must not be empty.
//
// This function updates architecture constraints in-place by:
// 1. Removing any existing kubernetes.io/arch matchExpressions from all terms
// 2. Adding the new architecture requirement to each term (or creating a new term if none exist)
// This approach avoids the Kubernetes API rejection: "no additions/deletions to non-empty NodeSelectorTerms list are allowed"
func applyArchitectureNodeAffinity(pod *corev1.Pod, architectures []string) {
	if len(architectures) == 0 {
		return
	}

	// Initialize affinity structure if needed
	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &corev1.Affinity{}
	}

	if pod.Spec.Affinity.NodeAffinity == nil {
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}

	// Create the architecture requirement
	architectureRequirement := corev1.NodeSelectorRequirement{
		Key:      utils.ArchLabel,
		Operator: corev1.NodeSelectorOpIn,
		Values:   architectures,
	}

	existingTerms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms

	// If no terms exist, create a single architecture-only term
	if len(existingTerms) == 0 {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{
			{
				MatchExpressions: []corev1.NodeSelectorRequirement{architectureRequirement},
			},
		}
		return
	}

	// Update in-place: remove old arch constraints and add new ones to each term
	// This preserves the NodeSelectorTerms array structure to avoid Kubernetes API rejection
	for i := range existingTerms {
		// Remove any existing architecture matchExpressions from this term
		var cleanedExpressions []corev1.NodeSelectorRequirement
		for _, expr := range existingTerms[i].MatchExpressions {
			if expr.Key != utils.ArchLabel {
				cleanedExpressions = append(cleanedExpressions, expr)
			}
		}

		// Add the new architecture requirement
		cleanedExpressions = append(cleanedExpressions, architectureRequirement)
		existingTerms[i].MatchExpressions = cleanedExpressions
	}

	// Reassign the updated terms for clarity and future maintainability
	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = existingTerms
}

// applyArchitectureConstraints applies architecture constraints to a pod by updating
// the node affinity in-place. It also removes architecture from nodeSelector if present.
// This ensures the plugin's architecture selection takes full effect while avoiding
// Kubernetes API rejections for NodeSelectorTerms modifications.
//
// Returns true if constraints were applied (architectures non-empty), false otherwise.
// Callers use the return value to decide whether this PPC "claimed" the pod: a false
// result means no architectures were produced and the caller should continue to the
// next PPC rather than treating this one as the first-match winner.
func applyArchitectureConstraints(pod *corev1.Pod, architectures []string) bool {
	if len(architectures) == 0 {
		return false
	}

	// Remove all existing architecture constraints first so that stale arch-only
	// NodeSelectorTerms (set by a previous MTO invocation) are cleaned up before
	// the new architecture is applied.  This avoids accumulating duplicate terms
	// across repeated invocations and ensures the fresh arch replaces the stale one.
	removeAllArchitectureConstraints(pod)

	// Apply the new architecture constraints (creates a new term or merges into
	// existing non-arch terms).
	applyArchitectureNodeAffinity(pod, architectures)

	// Always return true because we always modify the pod by applying architecture constraints
	return true
}
