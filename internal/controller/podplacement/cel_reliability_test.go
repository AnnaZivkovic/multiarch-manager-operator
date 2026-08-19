/*
Copyright 2026 Red Hat, Inc.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/multiarch-tuning-operator/api/common/plugins"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

var _ = Describe("CEL Reliability", func() {

	// TestIdempotentRepeatedReconcile
	It("should produce stable pod state when repeatedly applying the same architectures (idempotent)", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					utils.ArchLabel: "amd64",
					"other-label":   "value",
				},
			},
		}
		architectures := []string{"ppc64le", "arm64"}
		for i := 0; i < 5; i++ {
			applyArchitectureConstraints(pod, architectures)

			if pod.Spec.NodeSelector != nil {
				Expect(pod.Spec.NodeSelector).NotTo(HaveKey(utils.ArchLabel),
					"Iteration %d: Architecture still in nodeSelector", i)
				Expect(pod.Spec.NodeSelector["other-label"]).To(Equal("value"),
					"Iteration %d: Other label was modified", i)
			}

			Expect(pod.Spec.Affinity).NotTo(BeNil(), "Iteration %d: Node affinity missing", i)
			Expect(pod.Spec.Affinity.NodeAffinity).NotTo(BeNil(), "Iteration %d: Node affinity missing", i)

			terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
			Expect(terms).To(HaveLen(1), "Iteration %d: Expected 1 term (idempotent)", i)
			Expect(terms[0].MatchExpressions).To(HaveLen(1), "Iteration %d", i)
			Expect(terms[0].MatchExpressions[0].Key).To(Equal(utils.ArchLabel), "Iteration %d", i)
			Expect(terms[0].MatchExpressions[0].Values).To(HaveLen(2), "Iteration %d", i)
		}
	})

	// TestArchitectureConstraintsReplacedInPlaceOnRepeatedApply
	It("should replace architecture constraints in-place on repeated apply", func() {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}

		applyArchitectureConstraints(pod, []string{"amd64"})
		Expect(pod.Spec.Affinity).NotTo(BeNil())
		terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1), "After first application")
		Expect(terms[0].MatchExpressions[0].Values[0]).To(Equal("amd64"))

		applyArchitectureConstraints(pod, []string{"ppc64le"})
		terms = pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1), "After second application (in-place replacement)")
		Expect(terms[0].MatchExpressions[0].Values[0]).To(Equal("ppc64le"))

		applyArchitectureConstraints(pod, []string{"arm64", "s390x"})
		terms = pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1), "After third application (in-place replacement)")
		Expect(terms[0].MatchExpressions).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(HaveLen(2))
	})

	// TestNodeSelectorCleanupStableAcrossMultipleReconciles
	It("should keep nodeSelector cleanup stable across multiple reconciles", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					utils.ArchLabel: "amd64",
					"zone":          "us-east-1",
					"tier":          "frontend",
				},
			},
		}

		for i := 0; i < 5; i++ {
			removed := removeArchitectureFromNodeSelector(pod)
			if i == 0 {
				Expect(removed).To(BeTrue(), "First cleanup should have removed architecture")
			} else {
				Expect(removed).To(BeFalse(), "Iteration %d: Cleanup should be idempotent", i)
			}
			Expect(pod.Spec.NodeSelector["zone"]).To(Equal("us-east-1"), "Iteration %d: zone label was modified", i)
			Expect(pod.Spec.NodeSelector["tier"]).To(Equal("frontend"), "Iteration %d: tier label was modified", i)
			Expect(pod.Spec.NodeSelector).NotTo(HaveKey(utils.ArchLabel), "Iteration %d: Architecture label still exists", i)
		}
	})

	// TestFallbackApplicationStable
	It("should keep fallback application stable across repeated runs", func() {
		rules := []plugins.ArchitectureRule{}
		fallback := []string{"amd64", "ppc64le"}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}

		for i := 0; i < 5; i++ {
			result, err := evaluateCELArchitecturePlacement(rules, fallback, pod)
			Expect(err).NotTo(HaveOccurred(), "Iteration %d", i)
			Expect(result.matched).To(BeFalse(), "Iteration %d", i)
			Expect(result.architectures).To(HaveLen(2), "Iteration %d", i)
		}
	})

	// TestNilAffinityHandling
	It("should handle nil affinity structures safely", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec:       corev1.PodSpec{Affinity: nil},
		}
		removed := removeArchitectureFromNodeAffinity(pod)
		Expect(removed).To(BeFalse(), "Should not report removal when affinity is nil")

		applyArchitectureNodeAffinity(pod, []string{"amd64"})
		Expect(pod.Spec.Affinity).NotTo(BeNil(), "Affinity should have been created")
	})
})
