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
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"github.com/openshift/multiarch-tuning-operator/api/common/plugins"
	"github.com/openshift/multiarch-tuning-operator/api/v1beta1"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

var _ = Describe("CEL Evaluator", func() {

	Describe("newCELEvaluator", func() {
		It("should create a valid CEL evaluator", func() {
			evaluator, err := newCELEvaluator()
			Expect(err).NotTo(HaveOccurred(), "Failed to create CEL evaluator")
			Expect(evaluator).NotTo(BeNil(), "CEL evaluator is nil")
			Expect(evaluator.env).NotTo(BeNil(), "CEL environment is nil")
			Expect(evaluator.cache).NotTo(BeNil(), "CEL cache is nil")
		})
	})

	Describe("CELEvaluator.compile", func() {
		var evaluator *celEvaluator

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred(), "Failed to create CEL evaluator")
		})

		DescribeTable("should compile expressions correctly",
			func(expression string, expectError bool) {
				_, err := evaluator.compile(expression)
				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("valid boolean expression",
				"self.metadata.name == 'test-pod'", false),
			Entry("valid label check with has() and map access",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'web'", false),
			Entry("exists() expression compiles with DynType",
				"self.metadata.labels.exists(l, l.key == 'app' && l.value == 'web')", false),
			Entry("invalid syntax",
				"self.metadata.name ==", true),
			Entry("non-boolean return type",
				"self.metadata.name", true),
			Entry("valid label check with bracket notation",
				"'app.kubernetes.io/component' in self.metadata.labels && self.metadata.labels['app.kubernetes.io/component'] == 'database'", false),
			Entry("missing label check returns false safely",
				"has(self.metadata.labels.nonexistent) && self.metadata.labels.nonexistent == 'value'", false),
		)

		It("should return cached program on second compile", func() {
			expression := "self.metadata.name == 'test'"
			prog1, err := evaluator.compile(expression)
			Expect(err).NotTo(HaveOccurred(), "First compile failed")
			prog2, err := evaluator.compile(expression)
			Expect(err).NotTo(HaveOccurred(), "Second compile failed")
			Expect(prog1).To(BeIdenticalTo(prog2), "Expected cached program to be returned")
		})
	})

	Describe("CELEvaluator.evaluate", func() {
		var evaluator *celEvaluator

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("should evaluate expressions correctly",
			func(expression string, pod *corev1.Pod, expectedResult bool, expectError bool) {
				result, err := evaluator.evaluate(expression, pod)
				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(expectedResult))
				}
			},
			Entry("match by name",
				"self.metadata.name == 'nginx-pod'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "nginx-pod"}},
				true, false),
			Entry("no match by name",
				"self.metadata.name == 'nginx-pod'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "redis-pod"}},
				false, false),
			Entry("match by label",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}}},
				true, false),
			Entry("name starts with",
				"self.metadata.name.startsWith('redis-')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "redis-master"}},
				true, false),
		)

		DescribeTable("negative cases",
			func(expression string, pod *corev1.Pod, expectError bool, description string) {
				_, err := evaluator.evaluate(expression, pod)
				if expectError {
					Expect(err).To(HaveOccurred(), description)
				} else {
					Expect(err).NotTo(HaveOccurred(), description)
				}
			},
			Entry("nil pod",
				"self.metadata.name == 'test'", nil, false,
				"Should handle nil pod gracefully"),
			Entry("empty expression",
				"", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, true,
				"Should reject empty expression"),
			Entry("malformed CEL syntax",
				"self.metadata.name ==", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, true,
				"Should reject malformed syntax"),
			Entry("undefined field access on DynType evaluator returns false, not error",
				"self.metadata.nonexistent == 'value'", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, false,
				"DynType runtime evaluator: unknown metadata field access does not error"),
			Entry("type mismatch errors at runtime",
				"self.metadata.name + 123", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, true,
				"Should detect type mismatches at runtime"),
			Entry("missing label key",
				"has(self.metadata.labels.nonexistent)", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, false,
				"Should handle missing label keys with has()"),
			Entry("nil labels map",
				"has(self.metadata.labels.app)", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: nil}}, false,
				"Should handle nil labels map"),
			Entry("empty labels map",
				"has(self.metadata.labels.app)", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{}}}, false,
				"Should handle empty labels map"),
			Entry("nil annotations map",
				"has(self.metadata.annotations.key)", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Annotations: nil}}, false,
				"Should handle nil annotations map"),
			Entry("special characters in name",
				"self.metadata.name == 'test-pod_123.example'", &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod_123.example"}}, false,
				"Should handle special characters in names"),
			Entry("unicode in labels",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'тест'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{"app": "тест"}}},
				false, "Should handle unicode in label values"),
			Entry("very long expression",
				"self.metadata.name == 'test' && self.metadata.name == 'test' && self.metadata.name == 'test' && self.metadata.name == 'test' && self.metadata.name == 'test'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, false,
				"Should handle long expressions"),
			Entry("complex boolean logic",
				"(self.metadata.name == 'test' || self.metadata.name == 'prod') && (has(self.metadata.labels.app) || has(self.metadata.labels.tier))",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{"app": "web"}}},
				false, "Should handle complex boolean logic"),
		)

		It("should be thread-safe for concurrent evaluation", func() {
			expression := "self.metadata.name.startsWith('test-')"
			errs := make(chan error, 10)
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
					_, err := evaluator.evaluate(expression, pod)
					if err != nil {
						errs <- err
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				Expect(err).NotTo(HaveOccurred(), "Concurrent evaluation error")
			}
		})
	})

	Describe("CELEvaluator.evaluateRules", func() {
		var evaluator *celEvaluator

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with postgres and redis rules", func() {
			var rules []plugins.ArchitectureRule

			BeforeEach(func() {
				rules = []plugins.ArchitectureRule{
					{
						Name:          "postgres-rule",
						Expression:    "self.metadata.name.startsWith('postgres-')",
						Architectures: []string{"ppc64le"},
					},
					{
						Name:          "redis-rule",
						Expression:    "self.metadata.name.startsWith('redis-')",
						Architectures: []string{"amd64", "ppc64le"},
					},
				}
			})

			It("should match first rule for postgres-db", func() {
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "postgres-db"}}
				rr := evaluator.evaluateRules(rules, pod)
				Expect(rr.matched).To(BeTrue())
				Expect(rr.architectures).To(ConsistOf("ppc64le"))
				Expect(rr.ruleName).To(Equal("postgres-rule"))
			})

			It("should match second rule for redis-cache", func() {
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "redis-cache"}}
				rr := evaluator.evaluateRules(rules, pod)
				Expect(rr.matched).To(BeTrue())
				Expect(rr.architectures).To(ConsistOf("amd64", "ppc64le"))
				Expect(rr.ruleName).To(Equal("redis-rule"))
			})

			It("should not match for nginx-web", func() {
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "nginx-web"}}
				rr := evaluator.evaluateRules(rules, pod)
				Expect(rr.matched).To(BeFalse())
			})
		})

		It("should return allErrored=true when all rules are malformed", func() {
			malformedRules := []plugins.ArchitectureRule{
				{Name: "bad1", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
				{Name: "bad2", Expression: "invalid syntax !!!!", Architectures: []string{"ppc64le"}},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			rr := evaluator.evaluateRules(malformedRules, pod)
			Expect(rr.allErrored).To(BeTrue(),
				"expected allErrored=true when all rules are malformed")
			Expect(rr.matched).To(BeFalse())
		})

		It("should return allErrored=false when some rules are valid (even if they don't match)", func() {
			rules := []plugins.ArchitectureRule{
				{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
				{Name: "valid-nomatch", Expression: "self.metadata.name == 'other'", Architectures: []string{"ppc64le"}},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			rr := evaluator.evaluateRules(rules, pod)
			Expect(rr.allErrored).To(BeFalse(),
				"expected allErrored=false when at least one valid rule exists")
			Expect(rr.matched).To(BeFalse())
		})

		It("should evaluate subsequent rules when first is malformed and second is valid and matches", func() {
			rules := []plugins.ArchitectureRule{
				{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
				{Name: "good", Expression: "self.metadata.name == 'test-pod'", Architectures: []string{"ppc64le"}},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			rr := evaluator.evaluateRules(rules, pod)
			Expect(rr.allErrored).To(BeFalse())
			Expect(rr.matched).To(BeTrue())
			Expect(rr.architectures).To(ConsistOf("ppc64le"))
		})
	})

	Describe("evaluateCELArchitecturePlacement", func() {
		DescribeTable("standard cases",
			func(rules []plugins.ArchitectureRule, fallback []string, pod *corev1.Pod,
				expectError bool, expectedArchs []string, expectedMatched bool) {
				result, err := evaluateCELArchitecturePlacement(rules, fallback, pod)
				if expectError {
					Expect(err).To(HaveOccurred())
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.matched).To(Equal(expectedMatched))
				Expect(result.architectures).To(HaveLen(len(expectedArchs)))
				for i, arch := range expectedArchs {
					Expect(result.architectures[i]).To(Equal(arch))
				}
			},
			Entry("rule matches",
				[]plugins.ArchitectureRule{{Name: "test-rule", Expression: "self.metadata.name == 'test-pod'", Architectures: []string{"ppc64le"}}},
				[]string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}},
				false, []string{"ppc64le"}, true),
			Entry("no rule matches, use fallback",
				[]plugins.ArchitectureRule{{Name: "test-rule", Expression: "self.metadata.name == 'other-pod'", Architectures: []string{"ppc64le"}}},
				[]string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}},
				false, []string{"amd64"}, false),
			Entry("no rules, use fallback",
				[]plugins.ArchitectureRule{},
				[]string{"amd64", "ppc64le"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}},
				false, []string{"amd64", "ppc64le"}, false),
		)

		DescribeTable("edge cases",
			func(rules []plugins.ArchitectureRule, fallback []string, pod *corev1.Pod,
				expectError bool, expectedArchs []string, expectedMatched bool, description string) {
				result, err := evaluateCELArchitecturePlacement(rules, fallback, pod)
				if expectError {
					Expect(err).To(HaveOccurred(), description)
					return
				}
				Expect(err).NotTo(HaveOccurred(), description)
				Expect(result).NotTo(BeNil(), description)
				Expect(result.matched).To(Equal(expectedMatched), description)
				Expect(result.architectures).To(HaveLen(len(expectedArchs)), description)
				for i, arch := range expectedArchs {
					Expect(result.architectures[i]).To(Equal(arch), description)
				}
			},
			Entry("nil rules and nil fallback",
				nil, nil, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				true, nil, false, "Should reject nil rules and fallback"),
			Entry("empty rules with fallback",
				[]plugins.ArchitectureRule{}, []string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				false, []string{"amd64"}, false, "Should use fallback with empty rules"),
			Entry("all rules fail to match",
				[]plugins.ArchitectureRule{
					{Name: "rule1", Expression: "self.metadata.name == 'nomatch1'", Architectures: []string{"ppc64le"}},
					{Name: "rule2", Expression: "self.metadata.name == 'nomatch2'", Architectures: []string{"s390x"}},
				},
				[]string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				false, []string{"amd64"}, false, "Should use fallback when no rules match"),
			Entry("first rule has invalid expression",
				[]plugins.ArchitectureRule{
					{Name: "invalid", Expression: "invalid syntax", Architectures: []string{"ppc64le"}},
					{Name: "valid", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64"}},
				},
				[]string{"s390x"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				false, []string{"amd64"}, true, "Should skip invalid rule and continue to next"),
			Entry("rule with empty architectures list",
				[]plugins.ArchitectureRule{{Name: "empty-arch", Expression: "self.metadata.name == 'test'", Architectures: []string{}}},
				[]string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				false, []string{}, true, "Should handle empty architectures list"),
			Entry("multiple architectures in single rule",
				[]plugins.ArchitectureRule{{Name: "multi-arch", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64", "arm64", "ppc64le", "s390x"}}},
				[]string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
				false, []string{"amd64", "arm64", "ppc64le", "s390x"}, true, "Should handle multiple architectures"),
			Entry("pod with no metadata",
				[]plugins.ArchitectureRule{{Name: "rule1", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64"}}},
				[]string{"ppc64le"},
				&corev1.Pod{},
				false, []string{"ppc64le"}, false, "Should handle pod with no metadata"),
			Entry("pod with empty name",
				[]plugins.ArchitectureRule{{Name: "rule1", Expression: "self.metadata.name == ''", Architectures: []string{"amd64"}}},
				[]string{"ppc64le"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ""}},
				false, []string{"amd64"}, true, "Should handle pod with empty name"),
		)

		It("should treat all-errored rules as non-matching and return fallback with allRulesErrored=true", func() {
			rules := []plugins.ArchitectureRule{
				{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
			}
			fallback := []string{"amd64"}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			result, err := evaluateCELArchitecturePlacement(rules, fallback, pod)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.allRulesErrored).To(BeTrue())
			Expect(result.matched).To(BeFalse())
			Expect(result.architectures).To(ConsistOf("amd64"))
		})

		It("should use fallback with empty rules and not set allErrored", func() {
			result, err := evaluateCELArchitecturePlacement(
				[]plugins.ArchitectureRule{},
				[]string{"amd64"},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.matched).To(BeFalse())
			Expect(result.architectures).To(ConsistOf("amd64"))
		})
	})

	Describe("validateCELExpression", func() {
		DescribeTable("should validate expressions",
			func(expression string, expectError bool) {
				err := validateCELExpression(expression)
				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("valid expression", "self.metadata.name == 'test'", false),
			Entry("invalid syntax", "self.metadata.name ==", true),
			Entry("non-boolean return", "self.metadata.name", true),
			Entry("valid map access expression",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'web'", false),
			Entry("valid bracket notation",
				"'app.kubernetes.io/component' in self.metadata.labels", false),
			Entry("exists() on dynamic map",
				"self.metadata.labels.exists(l, l.key == 'app')", false),
			Entry("incomplete expression",
				"self.metadata.name ==", true),
			Entry("valid name check",
				"self.metadata.name.startsWith('postgres-')", false),
		)
	})

	Describe("CEL map access syntax", func() {
		var evaluator *celEvaluator
		var pod *corev1.Pod

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred())
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app":                         "web",
						"tier":                        "frontend",
						"app.kubernetes.io/component": "database",
						"app.kubernetes.io/part-of":   "wordpress",
					},
					Annotations: map[string]string{
						"description": "test annotation",
					},
				},
			}
		})

		DescribeTable("should handle map access correctly",
			func(expression string, expectedResult bool, expectError bool, description string) {
				result, err := evaluator.evaluate(expression, pod)
				if expectError {
					Expect(err).To(HaveOccurred(), description)
					return
				}
				Expect(err).NotTo(HaveOccurred(), description)
				Expect(result).To(Equal(expectedResult), description)
			},
			Entry("simple label check with has()",
				"has(self.metadata.labels.app)", true, false,
				"Verify has() works for checking label existence"),
			Entry("label check with has() and value comparison",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'web'", true, false,
				"Verify has() + value check works"),
			Entry("missing label with has() returns false",
				"has(self.metadata.labels.nonexistent)", false, false,
				"Verify has() returns false for missing labels"),
			Entry("bracket notation for labels with dots",
				"'app.kubernetes.io/component' in self.metadata.labels && self.metadata.labels['app.kubernetes.io/component'] == 'database'",
				true, false, "Verify bracket notation works for labels with special characters"),
			Entry("multiple label checks with logical AND",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'web' && has(self.metadata.labels.tier) && self.metadata.labels.tier == 'frontend'",
				true, false, "Verify multiple label checks work"),
			Entry("annotation check with has()",
				"has(self.metadata.annotations.description) && self.metadata.annotations.description == 'test annotation'",
				true, false, "Verify annotations work the same as labels"),
			Entry(".exists() on labels compiles with DynType",
				"self.metadata.labels.exists(l, l.key == 'app')", false, false,
				"DynType runtime evaluator: .exists() on a dynamic map compiles without error"),
			Entry("name check with startsWith()",
				"self.metadata.name.startsWith('test-')", true, false,
				"Verify string methods work on metadata fields"),
			Entry("complex expression with OR logic",
				"(has(self.metadata.labels.app) && self.metadata.labels.app == 'web') || (has(self.metadata.labels.tier) && self.metadata.labels.tier == 'backend')",
				true, false, "Verify OR logic works correctly"),
		)
	})

	Describe("real-world CEL expression scenarios", func() {
		var evaluator *celEvaluator

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("should match production scenarios",
			func(expression string, pod *corev1.Pod, expectedResult bool, description string) {
				result, err := evaluator.evaluate(expression, pod)
				Expect(err).NotTo(HaveOccurred(), description)
				Expect(result).To(Equal(expectedResult), description)
			},
			Entry("operator namespace - openshift-operators",
				"self.metadata.namespace == 'openshift-operators'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "operator-pod", Namespace: "openshift-operators"}},
				true, "Should match pods in openshift-operators namespace"),
			Entry("well-known label - app component",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'database'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Labels: map[string]string{"app": "database"}}},
				true, "Should match app label"),
			Entry("well-known label - component",
				"has(self.metadata.labels.component) && self.metadata.labels.component == 'postgresql'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "postgres-pod", Labels: map[string]string{"component": "postgresql"}}},
				true, "Should match component label"),
			Entry("combined labels - app and component",
				"has(self.metadata.labels.app) && self.metadata.labels.app == 'database' && has(self.metadata.labels.component) && self.metadata.labels.component == 'postgresql'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "postgres-db", Labels: map[string]string{"app": "database", "component": "postgresql"}}},
				true, "Should match multiple labels combined"),
			Entry("tier and environment labels",
				"has(self.metadata.labels.tier) && self.metadata.labels.tier == 'frontend' && has(self.metadata.labels.environment) && self.metadata.labels.environment == 'production'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-prod", Labels: map[string]string{"tier": "frontend", "environment": "production"}}},
				true, "Should match tier and environment labels"),
			Entry("priority label - critical",
				"has(self.metadata.labels.priority) && self.metadata.labels.priority == 'critical'",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "critical-service", Labels: map[string]string{"priority": "critical"}}},
				true, "Should match priority label"),
			Entry("OR condition - backend with gold SLA",
				"has(self.metadata.labels.priority) && self.metadata.labels.priority == 'critical' || (has(self.metadata.labels.tier) && self.metadata.labels.tier == 'backend' && has(self.metadata.labels.sla) && self.metadata.labels.sla == 'gold')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "backend-gold", Labels: map[string]string{"tier": "backend", "sla": "gold"}}},
				true, "Should match OR condition with multiple labels"),
			Entry("name prefix - redis pods",
				"self.metadata.name.startsWith('redis-')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "redis-master-0"}},
				true, "Should match name prefix for StatefulSet pods"),
			Entry("name contains pattern",
				"self.metadata.name.contains('database')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-database-pod-123"}},
				true, "Should match name containing pattern"),
			Entry("namespace prefix match",
				"self.metadata.namespace.startsWith('prod-')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "prod-apps"}},
				true, "Should match namespace prefix"),
			Entry("label exists check only",
				"has(self.metadata.labels.migrationReady)",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "migrating-pod", Labels: map[string]string{"migrationReady": "true"}}},
				true, "Should check if label exists"),
			Entry("label does not exist",
				"!has(self.metadata.labels.legacy)",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "modern-pod", Labels: map[string]string{}}},
				true, "Should match when label does not exist"),
		)
	})

	Describe("podToMap", func() {
		DescribeTable("should expose correct metadata fields",
			func(pod *corev1.Pod, wantMeta map[string]interface{}) {
				m := podToMap(pod)
				meta, ok := m["metadata"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "metadata is not map[string]interface{}")
				for key, want := range wantMeta {
					got, exists := meta[key]
					Expect(exists).To(BeTrue(), "metadata[%q] missing", key)
					wantMap, wantIsMap := want.(map[string]interface{})
					gotMap, gotIsMap := got.(map[string]interface{})
					if wantIsMap && gotIsMap {
						Expect(len(gotMap)).To(Equal(len(wantMap)), "metadata[%q] map length mismatch", key)
					} else {
						Expect(got).To(Equal(want), "metadata[%q] value mismatch", key)
					}
				}
			},
			Entry("nil pod returns empty metadata with generateName",
				nil,
				map[string]interface{}{
					"name": "", "generateName": "", "namespace": "",
					"labels": map[string]interface{}{}, "annotations": map[string]interface{}{},
				}),
			Entry("pod with generateName",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "", GenerateName: "worker-", Namespace: "default"}},
				map[string]interface{}{
					"name": "", "generateName": "worker-", "namespace": "default",
					"labels": map[string]interface{}{}, "annotations": map[string]interface{}{},
				}),
			Entry("pod with name and no generateName",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "ns"}},
				map[string]interface{}{
					"name": "my-pod", "generateName": "", "namespace": "ns",
					"labels": map[string]interface{}{}, "annotations": map[string]interface{}{},
				}),
		)
	})

	Describe("generateName in CEL expressions", func() {
		var evaluator *celEvaluator

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("should match generateName correctly",
			func(expression string, pod *corev1.Pod, expectedResult bool) {
				result, err := evaluator.evaluate(expression, pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
			},
			Entry("match by generateName prefix",
				"self.metadata.generateName.startsWith('worker-')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "worker-"}},
				true),
			Entry("no match when generateName differs",
				"self.metadata.generateName.startsWith('worker-')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "redis-"}},
				false),
			Entry("empty generateName does not match prefix",
				"self.metadata.generateName.startsWith('worker-')",
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "explicit-name"}},
				false),
		)
	})

	Describe("first-match-wins and priority ordering", func() {
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
	})

	Describe("invalid CEL expression handling", func() {
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
	})

	Describe("concurrent CEL compilation and cache reuse", func() {
		var evaluator *celEvaluator

		BeforeEach(func() {
			var err error
			evaluator, err = newCELEvaluator()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be thread-safe for concurrent compilation", func() {
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

		It("should reuse cached compiled expressions", func() {
			expression := "self.metadata.name == 'test'"
			prog1, err := evaluator.compile(expression)
			Expect(err).NotTo(HaveOccurred())
			prog2, err := evaluator.compile(expression)
			Expect(err).NotTo(HaveOccurred())
			Expect(prog1).To(BeIdenticalTo(prog2), "Expected cached program to be reused")

			evaluator.mu.Lock()
			found := evaluator.cache.Contains(expression)
			evaluator.mu.Unlock()
			Expect(found).To(BeTrue(), "Expression not found in cache")
		})
	})

	Describe("nil and empty pod handling", func() {
		It("should handle nil pod without panicking or erroring", func() {
			rules := []plugins.ArchitectureRule{
				{Name: "test-rule", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64"}},
			}
			result, err := evaluateCELArchitecturePlacement(rules, []string{"ppc64le"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.matched).To(BeFalse())
			Expect(result.architectures).To(ConsistOf("ppc64le"))
		})

		It("should not modify pod for empty architectures list", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			modified := applyArchitectureConstraints(pod, []string{})
			Expect(modified).To(BeFalse())
			Expect(pod.Spec.Affinity).To(BeNil())
		})

		It("should use fallback for empty rules list", func() {
			rules := []plugins.ArchitectureRule{}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}
			result, err := evaluateCELArchitecturePlacement(rules, []string{"amd64", "arm64"}, pod)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.matched).To(BeFalse())
			Expect(result.architectures).To(HaveLen(2))
		})
	})

	Describe("applyCELInWebhook – malformed CEL and fallback interaction", func() {
		var ctx context.Context
		var recorder *record.FakeRecorder

		BeforeEach(func() {
			ctx = context.Background()
			recorder = record.NewFakeRecorder(8)
		})

		It("should skip malformed high-priority PPC and apply valid lower-priority PPC", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-workload", Namespace: "default"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx:latest"}}}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "high-prio-malformed", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 200,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{{Name: "malformed", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}}},
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "low-prio-valid", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"s390x"},
							Rules: []plugins.ArchitectureRule{{Name: "match-by-name", Expression: "self.metadata.name == 'my-workload'", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

			Expect(wrappedPod.Spec.Affinity).NotTo(BeNil())
			Expect(wrappedPod.Spec.Affinity.NodeAffinity).NotTo(BeNil())
			Expect(wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).NotTo(BeNil())
			var foundArchs []string
			for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
				for _, expr := range term.MatchExpressions {
					if expr.Key == utils.ArchLabel {
						foundArchs = append(foundArchs, expr.Values...)
					}
				}
			}
			Expect(foundArchs).To(ConsistOf("ppc64le"),
				"If amd64 appears here, the malformed high-priority PPC's fallback was incorrectly applied")
		})

		It("should apply fallback when CEL expression evaluates to false (valid but non-matching)", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unmatched-pod", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ppc-no-match", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"arm64"},
							Rules: []plugins.ArchitectureRule{{Name: "no-match", Expression: "self.metadata.name == 'other-pod'", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			var foundArchs []string
			if wrappedPod.Spec.Affinity != nil && wrappedPod.Spec.Affinity.NodeAffinity != nil &&
				wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
					for _, expr := range term.MatchExpressions {
						if expr.Key == utils.ArchLabel {
							foundArchs = append(foundArchs, expr.Values...)
						}
					}
				}
			}
			Expect(foundArchs).To(ConsistOf("arm64"))
		})

		It("should apply rule architectures when CEL expression evaluates to true", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "matched-pod", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ppc-match", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{{Name: "match", Expression: "self.metadata.name == 'matched-pod'", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			var foundArchs []string
			if wrappedPod.Spec.Affinity != nil && wrappedPod.Spec.Affinity.NodeAffinity != nil &&
				wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
					for _, expr := range term.MatchExpressions {
						if expr.Key == utils.ArchLabel {
							foundArchs = append(foundArchs, expr.Values...)
						}
					}
				}
			}
			Expect(foundArchs).To(ConsistOf("ppc64le"))
		})

		It("should evaluate lower-priority PPC when first has all-errored CEL", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "p300-malformed", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 200,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{{Name: "bad", Expression: "!!! invalid", Architectures: []string{"amd64"}}},
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "p100-valid", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"arm64"},
							Rules: []plugins.ArchitectureRule{{Name: "match", Expression: "self.metadata.name == 'app-pod'", Architectures: []string{"s390x"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			var foundArchs []string
			if wrappedPod.Spec.Affinity != nil && wrappedPod.Spec.Affinity.NodeAffinity != nil &&
				wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
					for _, expr := range term.MatchExpressions {
						if expr.Key == utils.ArchLabel {
							foundArchs = append(foundArchs, expr.Values...)
						}
					}
				}
			}
			Expect(foundArchs).To(ConsistOf("s390x"))
		})

		It("should not set arch constraint when all CEL rules are malformed and no PPC claims the pod", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "no-match-pod", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "all-malformed", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			if wrappedPod.Spec.Affinity != nil {
				if na := wrappedPod.Spec.Affinity.NodeAffinity; na != nil {
					if req := na.RequiredDuringSchedulingIgnoredDuringExecution; req != nil {
						Expect(req.NodeSelectorTerms).To(BeEmpty(),
							"expected no arch constraint when all CEL rules are malformed")
					}
				}
			}
		})
	})

	Describe("NodeAffinityLabel after applyCELInWebhook", func() {
		var ctx context.Context
		var recorder *record.FakeRecorder

		BeforeEach(func() {
			ctx = context.Background()
			recorder = record.NewFakeRecorder(8)
		})

		It("should set NodeAffinityLabel to 'overriden' when CEL successfully applies constraints", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "target-pod", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ppc-cel-match", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{{Name: "match", Expression: "self.metadata.name == 'target-pod'", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			Expect(wrappedPod.Labels[utils.NodeAffinityLabel]).To(Equal(utils.NodeAffinityLabelValueOverriden))
		})

		It("should NOT set NodeAffinityLabel to 'overriden' when all CEL rules fail", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "error-pod", Namespace: "default",
				Labels: map[string]string{utils.NodeAffinityLabel: utils.LabelValueNotSet},
			}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ppc-malformed", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			Expect(wrappedPod.Labels[utils.NodeAffinityLabel]).NotTo(Equal(utils.NodeAffinityLabelValueOverriden))
		})

		It("should set NodeAffinityLabel to 'overriden' when CEL fallback is applied", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "nomatch-pod", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ppc-fallback", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin: plugins.BasePlugin{Enabled: true}, FallbackArchitectures: []string{"s390x"},
							Rules: []plugins.ArchitectureRule{{Name: "no-match", Expression: "self.metadata.name == 'other'", Architectures: []string{"ppc64le"}}},
						}},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, ctx, recorder)
			wh.applyCELInWebhook(ctx, wrappedPod, ppcs)
			Expect(wrappedPod.Labels[utils.NodeAffinityLabel]).To(Equal(utils.NodeAffinityLabelValueOverriden))
		})
	})

	Describe("applyCELArchitecturePlacement – controller path", func() {
		// Both webhook and controller skip PPCs when all CEL rules fail:
		//   malformed CEL → allRulesErrored=true → skip PPC, do not apply fallback

		It("should skip PPC and return false when all CEL rules are malformed", func() {
			// Construct a minimal PodReconciler; only the Recorder field is needed
			// because applyCELArchitecturePlacement does not call the API server.
			recorder := record.NewFakeRecorder(8)
			reconciler := &PodReconciler{Recorder: recorder}

			ppc := v1beta1.PodPlacementConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "malformed-ppc", Namespace: "default"},
				Spec: v1beta1.PodPlacementConfigSpec{
					Plugins: &plugins.LocalPlugins{
						CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
							BasePlugin:            plugins.BasePlugin{Enabled: true},
							FallbackArchitectures: []string{"amd64"},
							Rules: []plugins.ArchitectureRule{
								{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}},
							},
						},
					},
				},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "default"}}
			wrappedPod := newPod(pod, context.Background(), recorder)

			handled := reconciler.applyCELArchitecturePlacement(context.Background(), ppc, wrappedPod)

			// The controller must NOT apply the fallback; skip the malformed PPC.
			Expect(handled).To(BeFalse(), "controller must skip malformed PPC without applying fallback")
			// Pod should have no affinity set by this PPC.
			if wrappedPod.Spec.Affinity != nil && wrappedPod.Spec.Affinity.NodeAffinity != nil {
				req := wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
				if req != nil {
					Expect(req.NodeSelectorTerms).To(BeEmpty(),
						"controller must not set NodeAffinity when all CEL rules are malformed")
				}
			}
		})

		It("should skip malformed PPC in both webhook and controller", func() {
			// Both code paths uniformly skip PPCs with all-malformed CEL rules.
			recorder := record.NewFakeRecorder(8)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "default"}}
			ppcs := []v1beta1.PodPlacementConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "malformed-ppc", Namespace: "default"},
					Spec: v1beta1.PodPlacementConfigSpec{
						Priority: 100,
						Plugins: &plugins.LocalPlugins{
							CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
								BasePlugin:            plugins.BasePlugin{Enabled: true},
								FallbackArchitectures: []string{"amd64"},
								Rules: []plugins.ArchitectureRule{
									{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"ppc64le"}},
								},
							},
						},
					},
				},
			}
			wh := &PodSchedulingGateMutatingWebHook{}
			wrappedPod := newPod(pod, context.Background(), recorder)
			wh.applyCELInWebhook(context.Background(), wrappedPod, ppcs)

			// Webhook must NOT have written any NodeAffinity for the malformed PPC.
			if wrappedPod.Spec.Affinity != nil && wrappedPod.Spec.Affinity.NodeAffinity != nil {
				req := wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
				if req != nil {
					Expect(req.NodeSelectorTerms).To(BeEmpty(),
						"webhook must not set NodeAffinity when the only PPC has all-malformed CEL rules")
				}
			}
		})
	})
})
