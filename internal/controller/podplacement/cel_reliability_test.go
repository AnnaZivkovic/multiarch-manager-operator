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
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/multiarch-tuning-operator/api/common/plugins"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

var _ = Describe("CEL Reliability", func() {

	// TestFirstMatchWinsStrictOrdering
	It("should evaluate rules strictly in order and stop at first match", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "first-rule", Expression: "self.metadata.name.startsWith('test-')", Architectures: []string{"ppc64le"}},
			{Name: "second-rule-also-matches", Expression: "self.metadata.name.startsWith('test-')", Architectures: []string{"amd64"}},
			{Name: "third-rule", Expression: "self.metadata.name == 'test-pod'", Architectures: []string{"arm64"}},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		result, err := evaluateCELArchitecturePlacement(rules, []string{"s390x"}, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.matched).To(BeTrue())
		Expect(result.ruleName).To(Equal("first-rule"))
		Expect(result.architectures).To(ConsistOf("ppc64le"))
	})

	// TestFirstMatchWinsFallbackNotApplied
	It("should not apply fallback when a rule matches", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "matching-rule", Expression: "self.metadata.name == 'test-pod'", Architectures: []string{"ppc64le"}},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		fallback := []string{"amd64", "arm64"}
		result, err := evaluateCELArchitecturePlacement(rules, fallback, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.matched).To(BeTrue())
		Expect(result.architectures).To(ConsistOf("ppc64le"))
	})

	// TestMultipleMatchingRulesOnlyFirstApplied
	It("should only apply first matching rule when multiple rules match", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "broad-match", Expression: "has(self.metadata.labels.app)", Architectures: []string{"ppc64le"}},
			{Name: "specific-match", Expression: "self.metadata.labels.app == 'web'", Architectures: []string{"amd64"}},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}}}
		result, err := evaluateCELArchitecturePlacement(rules, []string{"s390x"}, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ruleName).To(Equal("broad-match"))
		Expect(result.architectures).To(ConsistOf("ppc64le"))
	})

	// TestInvalidCELExpressionDoesNotPanic
	It("should not panic on invalid CEL expressions", func() {
		Expect(func() {
			rules := []plugins.ArchitectureRule{
				{Name: "invalid-syntax", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}},
				{Name: "valid-rule", Expression: "self.metadata.name == 'test-pod'", Architectures: []string{"amd64"}},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			result, err := evaluateCELArchitecturePlacement(rules, []string{"s390x"}, pod)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.matched).To(BeTrue())
			Expect(result.ruleName).To(Equal("valid-rule"))
		}).NotTo(Panic())
	})

	// TestInvalidCELTreatedAsFalse
	It("should treat invalid CEL as false (non-matching) and use fallback", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "invalid-expression", Expression: "self.nonexistent.field.access", Architectures: []string{"ppc64le"}},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		result, err := evaluateCELArchitecturePlacement(rules, []string{"amd64"}, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.matched).To(BeFalse())
		Expect(result.architectures).To(ConsistOf("amd64"))
	})

	// TestAllInvalidRulesTriggerFallback
	It("should use fallback when all rules are invalid", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "invalid-1", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}},
			{Name: "invalid-2", Expression: "self.nonexistent.field", Architectures: []string{"arm64"}},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		result, err := evaluateCELArchitecturePlacement(rules, []string{"amd64", "s390x"}, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.matched).To(BeFalse())
		Expect(result.architectures).To(HaveLen(2))
	})

	// TestRepeatedInvalidCELEvaluationStable
	It("should remain stable across repeated evaluations of invalid CEL", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "invalid-rule", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		for i := 0; i < 10; i++ {
			result, err := evaluateCELArchitecturePlacement(rules, []string{"amd64"}, pod)
			Expect(err).NotTo(HaveOccurred(), "Iteration %d", i)
			Expect(result.matched).To(BeFalse(), "Iteration %d", i)
			Expect(result.architectures).To(ConsistOf("amd64"), "Iteration %d", i)
		}
	})

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

	// TestConcurrentCELCompilation
	It("should be thread-safe for concurrent CEL compilation", func() {
		evaluator, err := newCELEvaluator()
		Expect(err).NotTo(HaveOccurred())

		expressions := []string{
			"self.metadata.name == 'test-1'",
			"self.metadata.name == 'test-2'",
			"self.metadata.name == 'test-3'",
			"self.metadata.name.startsWith('test-')",
			"has(self.metadata.labels.app)",
		}
		var wg sync.WaitGroup
		errors := make(chan error, len(expressions)*10)
		for i := 0; i < 10; i++ {
			for _, expr := range expressions {
				wg.Add(1)
				go func(expression string) {
					defer wg.Done()
					_, err := evaluator.compile(expression)
					if err != nil {
						errors <- err
					}
				}(expr)
			}
		}
		wg.Wait()
		close(errors)
		for err := range errors {
			Expect(err).NotTo(HaveOccurred(), "Concurrent compilation error")
		}
	})

	// TestCELCacheReuse
	It("should reuse cached compiled expressions", func() {
		evaluator, err := newCELEvaluator()
		Expect(err).NotTo(HaveOccurred())
		expression := "self.metadata.name == 'test'"

		prog1, err := evaluator.compile(expression)
		Expect(err).NotTo(HaveOccurred())
		prog2, err := evaluator.compile(expression)
		Expect(err).NotTo(HaveOccurred())
		Expect(prog1).To(BeIdenticalTo(prog2), "Expected cached program to be reused, but got different instance")

		evaluator.mu.Lock()
		found := evaluator.cache.Contains(expression)
		evaluator.mu.Unlock()
		Expect(found).To(BeTrue(), "Expression not found in cache")
	})

	// TestNilPodHandling
	It("should handle nil pod safely", func() {
		rules := []plugins.ArchitectureRule{
			{Name: "test-rule", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64"}},
		}
		result, err := evaluateCELArchitecturePlacement(rules, []string{"ppc64le"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.matched).To(BeFalse())
		Expect(result.architectures).To(ConsistOf("ppc64le"))
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

	// TestEmptyRulesUseFallback
	It("should use fallback for empty rules list", func() {
		rules := []plugins.ArchitectureRule{}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		result, err := evaluateCELArchitecturePlacement(rules, []string{"amd64", "arm64"}, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.matched).To(BeFalse())
		Expect(result.architectures).To(HaveLen(2))
	})

	// TestEmptyArchitecturesList
	It("should not modify pod for empty architectures list", func() {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
		modified := applyArchitectureConstraints(pod, []string{})
		Expect(modified).To(BeFalse(), "Empty architectures should not modify pod")
		if pod.Spec.Affinity != nil {
			Expect(pod.Spec.Affinity.NodeAffinity).To(BeNil(),
				"Node affinity should not be created for empty architectures")
		}
	})
})
