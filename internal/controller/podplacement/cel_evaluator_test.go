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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"github.com/openshift/multiarch-tuning-operator/api/common/plugins"
	"github.com/openshift/multiarch-tuning-operator/api/v1beta1"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

func TestNewCELEvaluator(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}
	if evaluator == nil {
		t.Fatal("CEL evaluator is nil")
	}
	if evaluator.env == nil {
		t.Fatal("CEL environment is nil")
	}
	if evaluator.cache == nil {
		t.Fatal("CEL cache is nil")
	}
}

func TestCELEvaluatorCompile(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	tests := []struct {
		name        string
		expression  string
		expectError bool
	}{
		{
			name:        "valid boolean expression",
			expression:  "self.metadata.name == 'test-pod'",
			expectError: false,
		},
		{
			name:        "valid label check with has() and map access",
			expression:  "has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
			expectError: false,
		},
		{
			// Runtime evaluator uses DynType.
			name:        "exists() expression compiles with DynType",
			expression:  "self.metadata.labels.exists(l, l.key == 'app' && l.value == 'web')",
			expectError: false,
		},
		{
			name:        "invalid syntax",
			expression:  "self.metadata.name ==",
			expectError: true,
		},
		{
			name:        "non-boolean return type",
			expression:  "self.metadata.name",
			expectError: true,
		},
		{
			name:        "valid label check with bracket notation",
			expression:  "'app.kubernetes.io/component' in self.metadata.labels && self.metadata.labels['app.kubernetes.io/component'] == 'database'",
			expectError: false,
		},
		{
			name:        "missing label check returns false safely",
			expression:  "has(self.metadata.labels.nonexistent) && self.metadata.labels.nonexistent == 'value'",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluator.compile(tt.expression)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestCELEvaluatorCompileCaching(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	expression := "self.metadata.name == 'test'"

	// First compilation
	prog1, err := evaluator.compile(expression)
	if err != nil {
		t.Fatalf("Failed to compile expression: %v", err)
	}

	// Second compilation should use cache
	prog2, err := evaluator.compile(expression)
	if err != nil {
		t.Fatalf("Failed to compile expression: %v", err)
	}

	// Should be the same program instance from cache
	if prog1 != prog2 {
		t.Error("Expected cached program to be returned")
	}
}

func TestCELEvaluatorEvaluate(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	tests := []struct {
		name           string
		expression     string
		pod            *corev1.Pod
		expectedResult bool
		expectError    bool
	}{
		{
			name:       "match by name",
			expression: "self.metadata.name == 'nginx-pod'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nginx-pod",
				},
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:       "no match by name",
			expression: "self.metadata.name == 'nginx-pod'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis-pod",
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:       "match by label",
			expression: "has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "web",
					},
				},
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:       "name starts with",
			expression: "self.metadata.name.startsWith('redis-')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis-master",
				},
			},
			expectedResult: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.evaluate(tt.expression, tt.pod)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expectedResult {
				t.Errorf("Expected result %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestCELEvaluatorEvaluateRules(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	rules := []plugins.ArchitectureRule{
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

	tests := []struct {
		name             string
		pod              *corev1.Pod
		expectedArchs    []string
		expectedRuleName string
		expectMatch      bool
	}{
		{
			name: "match first rule",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "postgres-db",
				},
			},
			expectedArchs:    []string{"ppc64le"},
			expectedRuleName: "postgres-rule",
			expectMatch:      true,
		},
		{
			name: "match second rule",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis-cache",
				},
			},
			expectedArchs:    []string{"amd64", "ppc64le"},
			expectedRuleName: "redis-rule",
			expectMatch:      true,
		},
		{
			name: "no match",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nginx-web",
				},
			},
			expectedArchs:    nil,
			expectedRuleName: "",
			expectMatch:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := evaluator.evaluateRules(rules, tt.pod)
			if tt.expectMatch {
				if !rr.matched {
					t.Error("Expected matched=true but got false")
				}
				if rr.architectures == nil {
					t.Error("Expected architectures but got nil")
				}
				if len(rr.architectures) != len(tt.expectedArchs) {
					t.Errorf("Expected %d architectures, got %d", len(tt.expectedArchs), len(rr.architectures))
				}
				if rr.ruleName != tt.expectedRuleName {
					t.Errorf("Expected rule name %s, got %s", tt.expectedRuleName, rr.ruleName)
				}
			} else {
				if rr.matched {
					t.Errorf("Expected no match but got matched=true with architectures: %v", rr.architectures)
				}
			}
		})
	}
}

func TestEvaluateCELArchitecturePlacement(t *testing.T) {
	tests := []struct {
		name                  string
		rules                 []plugins.ArchitectureRule
		fallbackArchitectures []string
		pod                   *corev1.Pod
		expectedArchs         []string
		expectedMatched       bool
		expectError           bool
	}{
		{
			name: "rule matches",
			rules: []plugins.ArchitectureRule{
				{
					Name:          "test-rule",
					Expression:    "self.metadata.name == 'test-pod'",
					Architectures: []string{"ppc64le"},
				},
			},
			fallbackArchitectures: []string{"amd64"},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			expectedArchs:   []string{"ppc64le"},
			expectedMatched: true,
			expectError:     false,
		},
		{
			name: "no rule matches, use fallback",
			rules: []plugins.ArchitectureRule{
				{
					Name:          "test-rule",
					Expression:    "self.metadata.name == 'other-pod'",
					Architectures: []string{"ppc64le"},
				},
			},
			fallbackArchitectures: []string{"amd64"},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			expectedArchs:   []string{"amd64"},
			expectedMatched: false,
			expectError:     false,
		},
		{
			name:                  "no rules, use fallback",
			rules:                 []plugins.ArchitectureRule{},
			fallbackArchitectures: []string{"amd64", "ppc64le"},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			expectedArchs:   []string{"amd64", "ppc64le"},
			expectedMatched: false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateCELArchitecturePlacement(tt.rules, tt.fallbackArchitectures, tt.pod)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != nil {
				if len(result.architectures) != len(tt.expectedArchs) {
					t.Errorf("Expected %d architectures, got %d", len(tt.expectedArchs), len(result.architectures))
				}
				if result.matched != tt.expectedMatched {
					t.Errorf("Expected matched=%v, got %v", tt.expectedMatched, result.matched)
				}
			}
		})
	}
}

func TestValidateCELExpression(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectError bool
	}{
		{
			name:        "valid expression",
			expression:  "self.metadata.name == 'test'",
			expectError: false,
		},
		{
			name:        "invalid syntax",
			expression:  "self.metadata.name ==",
			expectError: true,
		},
		{
			name:        "non-boolean return",
			expression:  "self.metadata.name",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCELExpression(tt.expression)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestCELEvaluatorNegativeCases tests error conditions and edge cases
func TestCELEvaluatorNegativeCases(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	tests := []struct {
		name        string
		expression  string
		pod         *corev1.Pod
		expectError bool
		description string
	}{
		{
			name:        "nil pod",
			expression:  "self.metadata.name == 'test'",
			pod:         nil,
			expectError: false, // We handle nil pods gracefully by returning empty metadata
			description: "Should handle nil pod gracefully",
		},
		{
			name:        "empty expression",
			expression:  "",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError: true,
			description: "Should reject empty expression",
		},
		{
			name:        "malformed CEL syntax",
			expression:  "self.metadata.name ==",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError: true,
			description: "Should reject malformed syntax",
		},
		{
			// Runtime evaluator uses DynType.
			// Schema validation happens during admission.
			name:        "undefined field access on DynType evaluator returns false, not error",
			expression:  "self.metadata.nonexistent == 'value'",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError: false,
			description: "DynType runtime evaluator: unknown metadata field access does not error",
		},
		{
			// Runtime evaluator uses DynType.
			name:        "type mismatch errors at runtime",
			expression:  "self.metadata.name + 123",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError: true,
			description: "Should detect type mismatches at runtime",
		},
		{
			name:        "missing label key",
			expression:  "has(self.metadata.labels.nonexistent)",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError: false,
			description: "Should handle missing label keys with has()",
		},
		{
			name:        "nil labels map",
			expression:  "has(self.metadata.labels.app)",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: nil}},
			expectError: false,
			description: "Should handle nil labels map",
		},
		{
			name:        "empty labels map",
			expression:  "has(self.metadata.labels.app)",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{}}},
			expectError: false,
			description: "Should handle empty labels map",
		},
		{
			name:        "nil annotations map",
			expression:  "has(self.metadata.annotations.key)",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Annotations: nil}},
			expectError: false,
			description: "Should handle nil annotations map",
		},
		{
			name:        "special characters in name",
			expression:  "self.metadata.name == 'test-pod_123.example'",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod_123.example"}},
			expectError: false,
			description: "Should handle special characters in names",
		},
		{
			name:        "unicode in labels",
			expression:  "has(self.metadata.labels.app) && self.metadata.labels.app == 'тест'",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{"app": "тест"}}},
			expectError: false,
			description: "Should handle unicode in label values",
		},
		{
			name:        "very long expression",
			expression:  "self.metadata.name == 'test' && self.metadata.name == 'test' && self.metadata.name == 'test' && self.metadata.name == 'test' && self.metadata.name == 'test'",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError: false,
			description: "Should handle long expressions",
		},
		{
			name:        "complex boolean logic",
			expression:  "(self.metadata.name == 'test' || self.metadata.name == 'prod') && (has(self.metadata.labels.app) || has(self.metadata.labels.tier))",
			pod:         &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{"app": "web"}}},
			expectError: false,
			description: "Should handle complex boolean logic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluator.evaluate(tt.expression, tt.pod)
			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}
		})
	}
}

// TestEvaluateCELArchitecturePlacementEdgeCases tests edge cases in rule evaluation
func TestEvaluateCELArchitecturePlacementEdgeCases(t *testing.T) {
	tests := []struct {
		name                  string
		rules                 []plugins.ArchitectureRule
		fallbackArchitectures []string
		pod                   *corev1.Pod
		expectError           bool
		expectedArchs         []string
		expectedMatched       bool
		description           string
	}{
		{
			name:                  "nil rules and nil fallback",
			rules:                 nil,
			fallbackArchitectures: nil,
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError:           true,
			description:           "Should reject nil rules and fallback",
		},
		{
			name:                  "empty rules with fallback",
			rules:                 []plugins.ArchitectureRule{},
			fallbackArchitectures: []string{"amd64"},
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError:           false,
			expectedArchs:         []string{"amd64"},
			expectedMatched:       false,
			description:           "Should use fallback with empty rules",
		},
		{
			name: "all rules fail to match",
			rules: []plugins.ArchitectureRule{
				{Name: "rule1", Expression: "self.metadata.name == 'nomatch1'", Architectures: []string{"ppc64le"}},
				{Name: "rule2", Expression: "self.metadata.name == 'nomatch2'", Architectures: []string{"s390x"}},
			},
			fallbackArchitectures: []string{"amd64"},
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError:           false,
			expectedArchs:         []string{"amd64"},
			expectedMatched:       false,
			description:           "Should use fallback when no rules match",
		},
		{
			name: "first rule has invalid expression",
			rules: []plugins.ArchitectureRule{
				{Name: "invalid", Expression: "invalid syntax", Architectures: []string{"ppc64le"}},
				{Name: "valid", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64"}},
			},
			fallbackArchitectures: []string{"s390x"},
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError:           false,
			expectedArchs:         []string{"amd64"},
			expectedMatched:       true,
			description:           "Should skip invalid rule and continue to next",
		},
		{
			name: "rule with empty architectures list",
			rules: []plugins.ArchitectureRule{
				{Name: "empty-arch", Expression: "self.metadata.name == 'test'", Architectures: []string{}},
			},
			fallbackArchitectures: []string{"amd64"},
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError:           false,
			expectedArchs:         []string{},
			expectedMatched:       true,
			description:           "Should handle empty architectures list",
		},
		{
			name: "multiple architectures in single rule",
			rules: []plugins.ArchitectureRule{
				{Name: "multi-arch", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64", "arm64", "ppc64le", "s390x"}},
			},
			fallbackArchitectures: []string{"amd64"},
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			expectError:           false,
			expectedArchs:         []string{"amd64", "arm64", "ppc64le", "s390x"},
			expectedMatched:       true,
			description:           "Should handle multiple architectures",
		},
		{
			name: "pod with no metadata",
			rules: []plugins.ArchitectureRule{
				{Name: "rule1", Expression: "self.metadata.name == 'test'", Architectures: []string{"amd64"}},
			},
			fallbackArchitectures: []string{"ppc64le"},
			pod:                   &corev1.Pod{},
			expectError:           false,
			expectedArchs:         []string{"ppc64le"},
			expectedMatched:       false,
			description:           "Should handle pod with no metadata",
		},
		{
			name: "pod with empty name",
			rules: []plugins.ArchitectureRule{
				{Name: "rule1", Expression: "self.metadata.name == ''", Architectures: []string{"amd64"}},
			},
			fallbackArchitectures: []string{"ppc64le"},
			pod:                   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ""}},
			expectError:           false,
			expectedArchs:         []string{"amd64"},
			expectedMatched:       true,
			description:           "Should handle pod with empty name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateCELArchitecturePlacement(tt.rules, tt.fallbackArchitectures, tt.pod)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
				return
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}
			if tt.expectError {
				return
			}

			if result == nil {
				t.Errorf("%s: result is nil", tt.description)
				return
			}

			if result.matched != tt.expectedMatched {
				t.Errorf("%s: expected matched=%v, got %v", tt.description, tt.expectedMatched, result.matched)
			}

			if len(result.architectures) != len(tt.expectedArchs) {
				t.Errorf("%s: expected %d architectures, got %d", tt.description, len(tt.expectedArchs), len(result.architectures))
				return
			}

			for i, arch := range tt.expectedArchs {
				if result.architectures[i] != arch {
					t.Errorf("%s: expected architecture[%d]=%s, got %s", tt.description, i, arch, result.architectures[i])
				}
			}
		})
	}
}

// TestCELEvaluatorConcurrency tests thread safety of CEL evaluator
func TestCELEvaluatorConcurrency(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	expression := "self.metadata.name.startsWith('test-')"

	// Run multiple goroutines concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			}
			_, err := evaluator.evaluate(expression, pod)
			if err != nil {
				t.Errorf("Goroutine %d: unexpected error: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestCELEvaluatorRealWorldScenarios tests real-world production scenarios from enhancement doc
func TestCELEvaluatorRealWorldScenarios(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	tests := []struct {
		name           string
		expression     string
		pod            *corev1.Pod
		expectedResult bool
		description    string
	}{
		{
			name:       "operator namespace - openshift-operators",
			expression: "self.metadata.namespace == 'openshift-operators'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "operator-pod",
					Namespace: "openshift-operators",
				},
			},
			expectedResult: true,
			description:    "Should match pods in openshift-operators namespace",
		},
		{
			name:       "well-known label - app component",
			expression: "has(self.metadata.labels.app) && self.metadata.labels.app == 'database'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "db-pod",
					Labels: map[string]string{
						"app": "database",
					},
				},
			},
			expectedResult: true,
			description:    "Should match app label",
		},
		{
			name:       "well-known label - component",
			expression: "has(self.metadata.labels.component) && self.metadata.labels.component == 'postgresql'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "postgres-pod",
					Labels: map[string]string{
						"component": "postgresql",
					},
				},
			},
			expectedResult: true,
			description:    "Should match component label",
		},
		{
			name:       "combined labels - app and component",
			expression: "has(self.metadata.labels.app) && self.metadata.labels.app == 'database' && has(self.metadata.labels.component) && self.metadata.labels.component == 'postgresql'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "postgres-db",
					Labels: map[string]string{
						"app":       "database",
						"component": "postgresql",
					},
				},
			},
			expectedResult: true,
			description:    "Should match multiple labels combined",
		},
		{
			name:       "tier and environment labels",
			expression: "has(self.metadata.labels.tier) && self.metadata.labels.tier == 'frontend' && has(self.metadata.labels.environment) && self.metadata.labels.environment == 'production'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "frontend-prod",
					Labels: map[string]string{
						"tier":        "frontend",
						"environment": "production",
					},
				},
			},
			expectedResult: true,
			description:    "Should match tier and environment labels",
		},
		{
			name:       "priority label - critical",
			expression: "has(self.metadata.labels.priority) && self.metadata.labels.priority == 'critical'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "critical-service",
					Labels: map[string]string{
						"priority": "critical",
					},
				},
			},
			expectedResult: true,
			description:    "Should match priority label",
		},
		{
			name:       "OR condition - backend with gold SLA",
			expression: "has(self.metadata.labels.priority) && self.metadata.labels.priority == 'critical' || (has(self.metadata.labels.tier) && self.metadata.labels.tier == 'backend' && has(self.metadata.labels.sla) && self.metadata.labels.sla == 'gold')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "backend-gold",
					Labels: map[string]string{
						"tier": "backend",
						"sla":  "gold",
					},
				},
			},
			expectedResult: true,
			description:    "Should match OR condition with multiple labels",
		},
		{
			name:       "name prefix - redis pods",
			expression: "self.metadata.name.startsWith('redis-')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis-master-0",
				},
			},
			expectedResult: true,
			description:    "Should match name prefix for StatefulSet pods",
		},
		{
			name:       "name contains pattern",
			expression: "self.metadata.name.contains('database')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-database-pod-123",
				},
			},
			expectedResult: true,
			description:    "Should match name containing pattern",
		},
		{
			name:       "namespace prefix match",
			expression: "self.metadata.namespace.startsWith('prod-')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-pod",
					Namespace: "prod-apps",
				},
			},
			expectedResult: true,
			description:    "Should match namespace prefix",
		},
		{
			name:       "label exists check only",
			expression: "has(self.metadata.labels.migrationReady)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "migrating-pod",
					Labels: map[string]string{
						"migrationReady": "true",
					},
				},
			},
			expectedResult: true,
			description:    "Should check if label exists",
		},
		{
			name:       "label does not exist",
			expression: "!has(self.metadata.labels.legacy)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "modern-pod",
					Labels: map[string]string{},
				},
			},
			expectedResult: true,
			description:    "Should match when label does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.evaluate(tt.expression, tt.pod)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
				return
			}
			if result != tt.expectedResult {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedResult, result)
			}
		})
	}
}

// TestCELMapAccessSyntax verifies that CEL expressions use correct map access syntax
// for labels and annotations, not the .exists() method which doesn't work on maps
func TestCELMapAccessSyntax(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	pod := &corev1.Pod{
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

	tests := []struct {
		name           string
		expression     string
		expectedResult bool
		expectError    bool
		description    string
	}{
		{
			name:           "simple label check with has()",
			expression:     "has(self.metadata.labels.app)",
			expectedResult: true,
			expectError:    false,
			description:    "Verify has() works for checking label existence",
		},
		{
			name:           "label check with has() and value comparison",
			expression:     "has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
			expectedResult: true,
			expectError:    false,
			description:    "Verify has() + value check works",
		},
		{
			name:           "missing label with has() returns false",
			expression:     "has(self.metadata.labels.nonexistent)",
			expectedResult: false,
			expectError:    false,
			description:    "Verify has() returns false for missing labels",
		},
		{
			name:           "bracket notation for labels with dots",
			expression:     "'app.kubernetes.io/component' in self.metadata.labels && self.metadata.labels['app.kubernetes.io/component'] == 'database'",
			expectedResult: true,
			expectError:    false,
			description:    "Verify bracket notation works for labels with special characters",
		},
		{
			name:           "multiple label checks with logical AND",
			expression:     "has(self.metadata.labels.app) && self.metadata.labels.app == 'web' && has(self.metadata.labels.tier) && self.metadata.labels.tier == 'frontend'",
			expectedResult: true,
			expectError:    false,
			description:    "Verify multiple label checks work",
		},
		{
			name:           "annotation check with has()",
			expression:     "has(self.metadata.annotations.description) && self.metadata.annotations.description == 'test annotation'",
			expectedResult: true,
			expectError:    false,
			description:    "Verify annotations work the same as labels",
		},
		{
			// Runtime evaluator uses DynType.
			// Schema validation happens during admission.
			name:        ".exists() on labels compiles with DynType (no static map-type checking)",
			expression:  "self.metadata.labels.exists(l, l.key == 'app')",
			expectError: false,
			description: "DynType runtime evaluator: .exists() on a dynamic map compiles without error",
		},
		{
			name:           "name check with startsWith()",
			expression:     "self.metadata.name.startsWith('test-')",
			expectedResult: true,
			expectError:    false,
			description:    "Verify string methods work on metadata fields",
		},
		{
			name:           "complex expression with OR logic",
			expression:     "(has(self.metadata.labels.app) && self.metadata.labels.app == 'web') || (has(self.metadata.labels.tier) && self.metadata.labels.tier == 'backend')",
			expectedResult: true,
			expectError:    false,
			description:    "Verify OR logic works correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.evaluate(tt.expression, pod)

			if tt.expectError {
				if err == nil {
					t.Errorf("%s: Expected error but got none", tt.description)
				}
				return
			}

			if err != nil {
				t.Errorf("%s: Unexpected error: %v", tt.description, err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("%s: Expected result %v, got %v", tt.description, tt.expectedResult, result)
			}
		})
	}
}

// TestCELExpressionValidation tests the CEL compilation helper used by runtime evaluator tests.
func TestCELExpressionValidation(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectError bool
		description string
	}{
		{
			name:        "valid map access expression",
			expression:  "has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
			expectError: false,
			description: "Valid expression should pass validation",
		},
		{
			name:        "valid bracket notation",
			expression:  "'app.kubernetes.io/component' in self.metadata.labels",
			expectError: false,
			description: "Bracket notation should be valid",
		},
		{
			// validateCELExpression compiles with DynType, so exists() on a map is accepted.
			name:        "exists() on dynamic map",
			expression:  "self.metadata.labels.exists(l, l.key == 'app')",
			expectError: false,
			description: "exists() compiles with DynType",
		},
		{
			name:        "incomplete expression",
			expression:  "self.metadata.name ==",
			expectError: true,
			description: "Incomplete expression should fail",
		},
		{
			name:        "non-boolean return",
			expression:  "self.metadata.name",
			expectError: true,
			description: "Non-boolean expression should fail",
		},
		{
			name:        "valid name check",
			expression:  "self.metadata.name.startsWith('postgres-')",
			expectError: false,
			description: "String method should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCELExpression(tt.expression)

			if tt.expectError && err == nil {
				t.Errorf("%s: Expected error but got none", tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: Unexpected error: %v", tt.description, err)
			}
		})
	}
}

// TestPodToMap verifies that podToMap exposes all expected metadata fields including generateName.
func TestPodToMap(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		wantMeta map[string]interface{}
	}{
		{
			name: "nil pod returns empty metadata with generateName",
			pod:  nil,
			wantMeta: map[string]interface{}{
				"name":         "",
				"generateName": "",
				"namespace":    "",
				"labels":       map[string]interface{}{},
				"annotations":  map[string]interface{}{},
			},
		},
		{
			name: "pod with generateName",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:         "",
					GenerateName: "worker-",
					Namespace:    "default",
				},
			},
			wantMeta: map[string]interface{}{
				"name":         "",
				"generateName": "worker-",
				"namespace":    "default",
				"labels":       map[string]interface{}{},
				"annotations":  map[string]interface{}{},
			},
		},
		{
			name: "pod with name and no generateName",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-pod",
					Namespace: "ns",
				},
			},
			wantMeta: map[string]interface{}{
				"name":         "my-pod",
				"generateName": "",
				"namespace":    "ns",
				"labels":       map[string]interface{}{},
				"annotations":  map[string]interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := podToMap(tt.pod)
			meta, ok := m["metadata"].(map[string]interface{})
			if !ok {
				t.Fatalf("metadata is not map[string]interface{}")
			}
			for key, want := range tt.wantMeta {
				got, exists := meta[key]
				if !exists {
					t.Errorf("metadata[%q] missing", key)
					continue
				}
				// For maps, compare length only (sufficient for these tests)
				wantMap, wantIsMap := want.(map[string]interface{})
				gotMap, gotIsMap := got.(map[string]interface{})
				if wantIsMap && gotIsMap {
					if len(wantMap) != len(gotMap) {
						t.Errorf("metadata[%q] map length: want %d, got %d", key, len(wantMap), len(gotMap))
					}
				} else if got != want {
					t.Errorf("metadata[%q]: want %q, got %q", key, want, got)
				}
			}
		})
	}
}

// TestCELEvaluateGenerateName verifies that generateName is accessible in CEL expressions.
func TestCELEvaluateGenerateName(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	tests := []struct {
		name           string
		expression     string
		pod            *corev1.Pod
		expectedResult bool
	}{
		{
			name:       "match by generateName prefix",
			expression: "self.metadata.generateName.startsWith('worker-')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "worker-",
				},
			},
			expectedResult: true,
		},
		{
			name:       "no match when generateName differs",
			expression: "self.metadata.generateName.startsWith('worker-')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "redis-",
				},
			},
			expectedResult: false,
		},
		{
			name:       "empty generateName does not match prefix",
			expression: "self.metadata.generateName.startsWith('worker-')",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "explicit-name",
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.evaluate(tt.expression, tt.pod)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.expectedResult {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

// ── Critical regression tests for malformed CEL / fallback interaction ────────

// TestEvaluateRules_AllRulesErrored_AllErroredTrue verifies that evaluateRules
// returns allErrored=true when every rule in the list fails to compile.
func TestEvaluateRules_AllRulesErrored_AllErroredTrue(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	malformedRules := []plugins.ArchitectureRule{
		{Name: "bad1", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
		{Name: "bad2", Expression: "invalid syntax !!!!", Architectures: []string{"ppc64le"}},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}

	rr := evaluator.evaluateRules(malformedRules, pod)

	if !rr.allErrored {
		t.Errorf("expected allErrored=true when all rules are malformed, got allErrored=false (matched=%v archs=%v)",
			rr.matched, rr.architectures)
	}
	if rr.matched {
		t.Errorf("expected matched=false when all rules are malformed, got matched=true")
	}
}

// TestEvaluateRules_SomeErroredSomeMatchedFalse_NotAllErrored verifies that when
// some rules error but at least one evaluates to false (no match), allErrored is
// false and fallback is the right outcome.
func TestEvaluateRules_SomeErroredSomeMatchedFalse_NotAllErrored(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	rules := []plugins.ArchitectureRule{
		{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
		// valid but does not match
		{Name: "valid-nomatch", Expression: "self.metadata.name == 'other'", Architectures: []string{"ppc64le"}},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}

	rr := evaluator.evaluateRules(rules, pod)

	if rr.allErrored {
		t.Errorf("expected allErrored=false when at least one valid rule exists, got allErrored=true")
	}
	if rr.matched {
		t.Errorf("expected matched=false since the valid rule does not match, got matched=true")
	}
}

// TestEvaluateCELArchitecturePlacement_MalformedCEL_ReturnsError verifies that
// evaluateCELArchitecturePlacement returns errAllCELRulesErrored when all rules
// fail to compile, so callers know to skip the PPC rather than apply fallback.
func TestEvaluateCELArchitecturePlacement_MalformedCEL_ReturnsError(t *testing.T) {
	rules := []plugins.ArchitectureRule{
		{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
	}
	fallback := []string{"amd64"}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}

	result, err := evaluateCELArchitecturePlacement(rules, fallback, pod)

	if err == nil {
		t.Fatalf("expected errAllCELRulesErrored but got nil error; result=%+v", result)
	}
	if err != errAllCELRulesErrored {
		t.Errorf("expected errAllCELRulesErrored specifically, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when all rules errored, got: %+v", result)
	}
}

// TestEvaluateCELArchitecturePlacement_MalformedHighPriority_ValidLowPriority is the
// critical scenario from the review:
//
//	PPC A (priority 200): INVALID CEL + fallback amd64
//	PPC B (priority 100): VALID CEL matching the pod → ppc64le
//
// Expected: PPC A is SKIPPED (errAllCELRulesErrored), PPC B is evaluated and
// returns ppc64le.  The fallback amd64 must NOT be applied.
//
// This test exercises applyCELInWebhook directly, which is the layer where the
// PPC priority loop lives.
func TestApplyCELInWebhook_MalformedHighPriority_ValidLowPriority_ExpectPpc64le(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-workload",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c", Image: "nginx:latest"}},
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			// High-priority PPC with a completely malformed CEL expression.
			// Its fallback (amd64) must NOT be applied.
			ObjectMeta: metav1.ObjectMeta{Name: "high-prio-malformed", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 200,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"amd64"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "malformed",
								Expression:    "self.metadata.name ==", // intentionally broken
								Architectures: []string{"amd64"},
							},
						},
					},
				},
			},
		},
		{
			// Lower-priority PPC with a valid CEL expression that matches the pod.
			ObjectMeta: metav1.ObjectMeta{Name: "low-prio-valid", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"s390x"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "match-by-name",
								Expression:    "self.metadata.name == 'my-workload'",
								Architectures: []string{"ppc64le"},
							},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	// The pod must have ppc64le — NOT amd64 (which would mean the malformed PPC's
	// fallback was incorrectly applied).
	if wrappedPod.Spec.Affinity == nil ||
		wrappedPod.Spec.Affinity.NodeAffinity == nil ||
		wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatal("expected node affinity to be set on pod after CEL evaluation")
	}

	var foundArchs []string
	for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Key == utils.ArchLabel {
				foundArchs = append(foundArchs, expr.Values...)
			}
		}
	}

	if len(foundArchs) != 1 || foundArchs[0] != "ppc64le" {
		t.Errorf("expected architecture [ppc64le] from lower-priority valid PPC, got %v\n"+
			"(If amd64 appears here, the malformed high-priority PPC's fallback was incorrectly applied.)",
			foundArchs)
	}
}

// TestApplyCELInWebhook_ValidHighPriority_False_FallbackApplied verifies that when
// a high-priority PPC's CEL expression evaluates to false (not an error), its
// fallback IS applied — this is the correct enhancement behavior.
func TestApplyCELInWebhook_ValidHighPriority_False_FallbackApplied(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unmatched-pod",
			Namespace: "default",
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ppc-no-match", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"arm64"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "no-match",
								Expression:    "self.metadata.name == 'other-pod'",
								Architectures: []string{"ppc64le"},
							},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	// Fallback arm64 must be applied because the CEL expression evaluated to false.
	var foundArchs []string
	if wrappedPod.Spec.Affinity != nil &&
		wrappedPod.Spec.Affinity.NodeAffinity != nil &&
		wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			for _, expr := range term.MatchExpressions {
				if expr.Key == utils.ArchLabel {
					foundArchs = append(foundArchs, expr.Values...)
				}
			}
		}
	}

	if len(foundArchs) != 1 || foundArchs[0] != "arm64" {
		t.Errorf("expected fallback architecture [arm64] when CEL expression evaluates to false, got %v", foundArchs)
	}
}

// TestApplyCELInWebhook_ValidHighPriority_True_RuleApplied verifies that when a
// high-priority PPC's CEL expression evaluates to true, its rule's architectures
// are applied and no fallback is used.
func TestApplyCELInWebhook_ValidHighPriority_True_RuleApplied(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "matched-pod",
			Namespace: "default",
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ppc-match", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"amd64"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "match",
								Expression:    "self.metadata.name == 'matched-pod'",
								Architectures: []string{"ppc64le"},
							},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	var foundArchs []string
	if wrappedPod.Spec.Affinity != nil &&
		wrappedPod.Spec.Affinity.NodeAffinity != nil &&
		wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			for _, expr := range term.MatchExpressions {
				if expr.Key == utils.ArchLabel {
					foundArchs = append(foundArchs, expr.Values...)
				}
			}
		}
	}

	if len(foundArchs) != 1 || foundArchs[0] != "ppc64le" {
		t.Errorf("expected architecture [ppc64le] from matching CEL rule, got %v", foundArchs)
	}
}

// TestApplyCELInWebhook_MultiplePPCsFirstErrors_LowerPriorityEvaluated verifies that
// when the first (highest-priority) PPC has all-errored CEL, subsequent PPCs are evaluated.
func TestApplyCELInWebhook_MultiplePPCsFirstErrors_LowerPriorityEvaluated(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-pod",
			Namespace: "default",
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		// Priority 300: all rules malformed → must be skipped entirely.
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p300-malformed", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 200,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"amd64"},
						Rules: []plugins.ArchitectureRule{
							{Name: "bad", Expression: "!!! invalid", Architectures: []string{"amd64"}},
						},
					},
				},
			},
		},
		// Priority 100: valid matching rule → s390x.
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p100-valid", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"arm64"},
						Rules: []plugins.ArchitectureRule{
							{Name: "match", Expression: "self.metadata.name == 'app-pod'", Architectures: []string{"s390x"}},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	var foundArchs []string
	if wrappedPod.Spec.Affinity != nil &&
		wrappedPod.Spec.Affinity.NodeAffinity != nil &&
		wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		for _, term := range wrappedPod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			for _, expr := range term.MatchExpressions {
				if expr.Key == utils.ArchLabel {
					foundArchs = append(foundArchs, expr.Values...)
				}
			}
		}
	}

	if len(foundArchs) != 1 || foundArchs[0] != "s390x" {
		t.Errorf("expected [s390x] from valid lower-priority PPC; got %v (malformed high-priority PPC may have blocked evaluation)", foundArchs)
	}
}

// TestApplyCELInWebhook_MalformedCEL_NoLowerPriorityMatch_NoArchSet verifies that
// when all PPCs have malformed CEL and none match, no architecture constraint is
// set on the pod (the pod falls through to global/image-based logic).
func TestApplyCELInWebhook_MalformedCEL_NoLowerPriorityMatch_NoArchSet(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-match-pod",
			Namespace: "default",
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "all-malformed", Namespace: "default"},
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
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	// No architecture constraint must be set; the pod must fall through to
	// image-based / global logic.
	if wrappedPod.Spec.Affinity != nil {
		if na := wrappedPod.Spec.Affinity.NodeAffinity; na != nil {
			if req := na.RequiredDuringSchedulingIgnoredDuringExecution; req != nil && len(req.NodeSelectorTerms) > 0 {
				t.Errorf("expected no arch constraint when all CEL rules are malformed and no PPC claims the pod, got affinity=%+v", wrappedPod.Spec.Affinity)
			}
		}
	}
}

// TestEvaluateRules_MultipleRulesOneMalformed_SubsequentEvaluated verifies that
// within a single PPC, when the first rule is malformed and the second is valid
// and matches, the second rule is used (soft failure model within a PPC).
func TestEvaluateRules_MultipleRulesOneMalformed_SubsequentEvaluated(t *testing.T) {
	evaluator, err := newCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	rules := []plugins.ArchitectureRule{
		// Malformed rule (compile error).
		{Name: "bad", Expression: "self.metadata.name ==", Architectures: []string{"amd64"}},
		// Valid rule that matches.
		{Name: "good", Expression: "self.metadata.name == 'test-pod'", Architectures: []string{"ppc64le"}},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod"}}

	rr := evaluator.evaluateRules(rules, pod)

	if rr.allErrored {
		t.Errorf("expected allErrored=false because the second rule is valid, got allErrored=true")
	}
	if !rr.matched {
		t.Errorf("expected matched=true from the second valid rule, got matched=false")
	}
	if len(rr.architectures) != 1 || rr.architectures[0] != "ppc64le" {
		t.Errorf("expected [ppc64le] from second rule, got %v", rr.architectures)
	}
}

// TestEvaluateCELArchitecturePlacement_EmptyRules_FallbackApplied verifies that
// when there are no rules (empty slice), fallback is applied correctly and
// allErrored is not set.
func TestEvaluateCELArchitecturePlacement_EmptyRules_FallbackApplied(t *testing.T) {
	result, err := evaluateCELArchitecturePlacement(
		[]plugins.ArchitectureRule{},
		[]string{"amd64"},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
	)
	if err != nil {
		t.Fatalf("expected no error for empty rules, got: %v", err)
	}
	if result.matched {
		t.Errorf("expected matched=false for empty rules")
	}
	if len(result.architectures) != 1 || result.architectures[0] != "amd64" {
		t.Errorf("expected fallback [amd64], got %v", result.architectures)
	}
}

// ── NodeAffinityLabel regression tests ───────────────────────────────────────

// TestApplyCELInWebhook_SuccessfulCEL_NodeAffinityLabelOverriden verifies that
// when CEL successfully applies architecture constraints, NodeAffinityLabel is
// set to "overriden" (intentional spelling per Paul's review).
func TestApplyCELInWebhook_SuccessfulCEL_NodeAffinityLabelOverriden(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-pod",
			Namespace: "default",
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ppc-cel-match", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"amd64"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "match",
								Expression:    "self.metadata.name == 'target-pod'",
								Architectures: []string{"ppc64le"},
							},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	// NodeAffinityLabel must be "overriden" when CEL successfully applied constraints.
	got := wrappedPod.Labels[utils.NodeAffinityLabel]
	if got != utils.NodeAffinityLabelValueOverriden {
		t.Errorf("expected NodeAffinityLabel=%q after successful CEL, got %q",
			utils.NodeAffinityLabelValueOverriden, got)
	}
}

// TestApplyCELInWebhook_CELError_NodeAffinityLabelNotOverriden verifies that when all
// CEL rules fail (malformed), NodeAffinityLabel is NOT set to "overriden".
// The pod falls through to image-based logic; the label should remain unset at
// this point (the webhook sets it to "not-set" before calling applyCELInWebhook,
// but applyCELInWebhook itself must not change it to "overriden").
func TestApplyCELInWebhook_CELError_NodeAffinityLabelNotOverriden(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-pod",
			Namespace: "default",
			Labels:    map[string]string{utils.NodeAffinityLabel: utils.LabelValueNotSet},
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ppc-malformed", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"amd64"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "bad",
								Expression:    "self.metadata.name ==", // compile error
								Architectures: []string{"ppc64le"},
							},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	// NodeAffinityLabel must NOT become "overriden" when all CEL rules errored.
	got := wrappedPod.Labels[utils.NodeAffinityLabel]
	if got == utils.NodeAffinityLabelValueOverriden {
		t.Errorf("expected NodeAffinityLabel != %q when all CEL rules fail, but got %q",
			utils.NodeAffinityLabelValueOverriden, got)
	}
}

// TestApplyCELInWebhook_CELFallback_NodeAffinityLabelOverriden verifies that when
// CEL rules evaluate to false (no match) and fallback architectures are applied,
// NodeAffinityLabel is ALSO set to "overriden" — fallback is a CEL-path outcome.
func TestApplyCELInWebhook_CELFallback_NodeAffinityLabelOverriden(t *testing.T) {
	ctx := context.Background()
	recorder := record.NewFakeRecorder(8)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nomatch-pod",
			Namespace: "default",
		},
	}

	ppcs := []v1beta1.PodPlacementConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ppc-fallback", Namespace: "default"},
			Spec: v1beta1.PodPlacementConfigSpec{
				Priority: 100,
				Plugins: &plugins.LocalPlugins{
					CelArchitecturePlacement: &plugins.CelArchitecturePlacement{
						BasePlugin:            plugins.BasePlugin{Enabled: true},
						FallbackArchitectures: []string{"s390x"},
						Rules: []plugins.ArchitectureRule{
							{
								Name:          "no-match",
								Expression:    "self.metadata.name == 'other'",
								Architectures: []string{"ppc64le"},
							},
						},
					},
				},
			},
		},
	}

	wh := &PodSchedulingGateMutatingWebHook{}
	wrappedPod := newPod(pod, ctx, recorder)
	wh.applyCELInWebhook(ctx, wrappedPod, ppcs)

	// Fallback s390x was applied via CEL path → label must be "overriden".
	got := wrappedPod.Labels[utils.NodeAffinityLabel]
	if got != utils.NodeAffinityLabelValueOverriden {
		t.Errorf("expected NodeAffinityLabel=%q after CEL fallback, got %q",
			utils.NodeAffinityLabelValueOverriden, got)
	}
}
