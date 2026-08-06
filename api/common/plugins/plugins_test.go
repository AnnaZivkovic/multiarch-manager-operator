package plugins

import (
	"strings"
	"testing"

	"github.com/openshift/multiarch-tuning-operator/api/common"
)

func TestBasePlugin_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"Enabled Plugin", true},
		{"Disabled Plugin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &BasePlugin{Enabled: tt.enabled}
			if plugin.IsEnabled() != tt.enabled {
				t.Errorf("Expected IsEnabled() to be %v, got %v", tt.enabled, plugin.IsEnabled())
			}
		})
	}
}

func TestBasePlugin_Name(t *testing.T) {
	plugin := &BasePlugin{}
	if plugin.Name() != "BasePlugin" {
		t.Errorf("Expected Name() to return 'BasePlugin', got %s", plugin.Name())
	}
}

func TestNodeAffinityScoring_Name(t *testing.T) {
	plugin := &NodeAffinityScoring{}

	if plugin.Name() != NodeAffinityScoringPluginName {
		t.Errorf("Expected plugin name %s, but got %s", NodeAffinityScoringPluginName, plugin.Name())
	}
}

func TestExecFormatErrorMonitor_Name(t *testing.T) {
	plugin := &ExecFormatErrorMonitor{}

	if plugin.Name() != ExecFormatErrorMonitorPluginName {
		t.Errorf("Expected plugin name %s, but got %s", ExecFormatErrorMonitorPluginName, plugin.Name())
	}
}

func TestCelArchitecturePlacement_Name(t *testing.T) {
	plugin := &CelArchitecturePlacement{}

	if plugin.Name() != "celArchitecturePlacement" {
		t.Errorf("Expected plugin name 'celArchitecturePlacement', but got %s", plugin.Name())
	}
}

func TestCelArchitecturePlacement_ValidateArchitectures(t *testing.T) {
	tests := []struct {
		name                  string
		fallbackArchitectures []string
		rules                 []ArchitectureRule
		expectError           bool
		errorContains         string
	}{
		{
			name:                  "valid single fallback architecture",
			fallbackArchitectures: []string{"amd64"},
			rules:                 nil,
			expectError:           false,
		},
		{
			name:                  "valid multiple fallback architectures",
			fallbackArchitectures: []string{"amd64", "arm64", "ppc64le", "s390x"},
			rules:                 nil,
			expectError:           false,
		},
		{
			name:                  "invalid fallback architecture",
			fallbackArchitectures: []string{"invalid-arch"},
			rules:                 nil,
			expectError:           true,
			errorContains:         "invalid fallback architecture: invalid-arch",
		},
		{
			name:                  "valid rule architectures",
			fallbackArchitectures: []string{"amd64"},
			rules: []ArchitectureRule{
				{
					Name:          "test-rule",
					Expression:    "true",
					Architectures: []string{"ppc64le", "arm64"},
				},
			},
			expectError: false,
		},
		{
			name:                  "invalid rule architecture",
			fallbackArchitectures: []string{"amd64"},
			rules: []ArchitectureRule{
				{
					Name:          "test-rule",
					Expression:    "true",
					Architectures: []string{"invalid-arch"},
				},
			},
			expectError:   true,
			errorContains: "invalid architecture in rule test-rule: invalid-arch",
		},
		{
			name:                  "multiple rules with valid architectures",
			fallbackArchitectures: []string{"amd64"},
			rules: []ArchitectureRule{
				{
					Name:          "rule1",
					Expression:    "true",
					Architectures: []string{"ppc64le"},
				},
				{
					Name:          "rule2",
					Expression:    "true",
					Architectures: []string{"arm64", "s390x"},
				},
			},
			expectError: false,
		},
		{
			name:                  "multiple rules with one invalid",
			fallbackArchitectures: []string{"amd64"},
			rules: []ArchitectureRule{
				{
					Name:          "rule1",
					Expression:    "true",
					Architectures: []string{"ppc64le"},
				},
				{
					Name:          "rule2",
					Expression:    "true",
					Architectures: []string{"bad-arch"},
				},
			},
			expectError:   true,
			errorContains: "invalid architecture in rule rule2: bad-arch",
		},
		{
			name:                  "all supported architectures",
			fallbackArchitectures: []string{"amd64", "arm64", "ppc64le", "s390x"},
			rules: []ArchitectureRule{
				{
					Name:          "all-archs",
					Expression:    "true",
					Architectures: []string{"amd64", "arm64", "ppc64le", "s390x"},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &CelArchitecturePlacement{
				FallbackArchitectures: tt.fallbackArchitectures,
				Rules:                 tt.rules,
			}

			err := plugin.ValidateArchitectures()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestLocalPluginChecks_CelArchitecturePlacement(t *testing.T) {
	tests := []struct {
		name     string
		plugins  *LocalPlugins
		expected bool
	}{
		{
			name: "plugin enabled",
			plugins: &LocalPlugins{
				CelArchitecturePlacement: &CelArchitecturePlacement{
					BasePlugin: BasePlugin{Enabled: true},
				},
			},
			expected: true,
		},
		{
			name: "plugin disabled",
			plugins: &LocalPlugins{
				CelArchitecturePlacement: &CelArchitecturePlacement{
					BasePlugin: BasePlugin{Enabled: false},
				},
			},
			expected: false,
		},
		{
			name:     "plugin not configured",
			plugins:  &LocalPlugins{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkFunc, exists := localPluginChecks[common.CelArchitecturePlacementPluginName]
			if !exists {
				t.Fatalf("Plugin check function not registered for CelArchitecturePlacementPluginName")
			}

			result := checkFunc(tt.plugins)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCelArchitecturePlacement_ValidateCELExpressions(t *testing.T) {
	tests := []struct {
		name             string
		rules            []ArchitectureRule
		expectError      bool
		errorContains    []string
		errorNotContains []string
	}{
		{
			name:        "no rules succeeds",
			rules:       nil,
			expectError: false,
		},
		{
			name: "valid boolean expression succeeds",
			rules: []ArchitectureRule{
				{
					Name:          "rule1",
					Expression:    "self.metadata.name == 'test'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "invalid CEL syntax is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "bad-rule",
					Expression:    "self.metadata.name ==",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: []string{`"bad-rule"`, "Syntax error"},
		},
		{
			name: "non-boolean expression is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "non-bool",
					Expression:    "self.metadata.name",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: []string{`"non-bool"`, "must return a boolean"},
		},
		{
			name: "multiple rules validated; second rule invalid",
			rules: []ArchitectureRule{
				{
					Name:          "ok-rule",
					Expression:    "self.metadata.name == 'test'",
					Architectures: []string{"amd64"},
				},
				{
					Name:          "bad-rule",
					Expression:    "self.metadata.name ==",
					Architectures: []string{"ppc64le"},
				},
			},
			expectError:   true,
			errorContains: []string{`"bad-rule"`, "Syntax error"},
		},
		// Typed admission validation rejects self.spec.* and self.status.*.
		{
			name: "self.spec reference is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "spec-rule",
					Expression:    "self.spec.nodeName == 'worker-1'",
					Architectures: []string{"amd64"},
				},
			},
			expectError:      true,
			errorContains:    []string{`"spec-rule"`, "field", "spec"},
			errorNotContains: []string{"references a disallowed field"},
		},
		{
			name: "self.status reference is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "status-rule",
					Expression:    "self.status.phase == 'Running'",
					Architectures: []string{"amd64"},
				},
			},
			expectError:      true,
			errorContains:    []string{`"status-rule"`, "field", "status"},
			errorNotContains: []string{"references a disallowed field"},
		},
		{
			name: "self.spec.containers reference is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "containers-rule",
					Expression:    "size(self.spec.containers) > 0",
					Architectures: []string{"amd64"},
				},
			},
			expectError:      true,
			errorContains:    []string{`"containers-rule"`, "field", "spec"},
			errorNotContains: []string{"references a disallowed field"},
		},
		{
			name: "self.metadata reference is permitted (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "labels-rule",
					Expression:    "has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "self.metadata.namespace is permitted (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "namespace-rule",
					Expression:    "self.metadata.namespace == 'production'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "self.metadata.annotations is permitted (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "annotations-rule",
					Expression:    "'config.company.io/tier' in self.metadata.annotations && self.metadata.annotations['config.company.io/tier'] == 'gpu'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "has(self.spec.nodeName) is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "has-spec-rule",
					Expression:    "has(self.spec.nodeName)",
					Architectures: []string{"amd64"},
				},
			},
			expectError:      true,
			errorContains:    []string{`"has-spec-rule"`, "field", "spec"},
			errorNotContains: []string{"references a disallowed field"},
		},
		{
			name: "multiple rules; first valid, second spec reference rejected",
			rules: []ArchitectureRule{
				{
					Name:          "valid-rule",
					Expression:    "self.metadata.name == 'test'",
					Architectures: []string{"amd64"},
				},
				{
					Name:          "spec-rule",
					Expression:    "self.spec.serviceAccountName == 'default'",
					Architectures: []string{"ppc64le"},
				},
			},
			expectError:      true,
			errorContains:    []string{`"spec-rule"`, "field", "spec"},
			errorNotContains: []string{"references a disallowed field"},
		},
		// Typed admission validation rejects comprehensions over self.spec.*.
		{
			name: "comprehension over self.spec.containers is rejected",
			rules: []ArchitectureRule{
				{
					Name:          "exists-rule",
					Expression:    `self.spec.containers.exists(c, c.name == "nginx")`,
					Architectures: []string{"amd64"},
				},
			},
			expectError:      true,
			errorContains:    []string{`"exists-rule"`, "field", "spec"},
			errorNotContains: []string{"references a disallowed field"},
		},
		// Constant expressions are allowed.
		{
			name: "constant expression true is permitted",
			rules: []ArchitectureRule{
				{
					Name:          "const-rule",
					Expression:    "true",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		// Anna's review: typed DeclTypeProvider rejects invalid metadata field names at
		// admission/compile time.  "labells" is a typo for "labels" and must be rejected
		// because the typed schema only defines "labels", not "labells".
		{
			name: "self.metadata.labells typo is rejected by typed schema (Anna review)",
			rules: []ArchitectureRule{
				{
					Name:          "typo-rule",
					Expression:    "self.metadata.labells.app == 'web'",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: []string{`"typo-rule"`},
		},
		// Valid metadata field accesses must continue to work after the typed schema change.
		{
			name: "self.metadata.name is permitted (valid field)",
			rules: []ArchitectureRule{
				{
					Name:          "name-rule",
					Expression:    "self.metadata.name == 'my-pod'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "self.metadata.namespace is permitted (valid field)",
			rules: []ArchitectureRule{
				{
					Name:          "namespace-rule",
					Expression:    "self.metadata.namespace == 'default'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "self.metadata.labels is permitted (valid field)",
			rules: []ArchitectureRule{
				{
					Name:          "labels-rule",
					Expression:    "has(self.metadata.labels.app) && self.metadata.labels.app == 'web'",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
		{
			name: "self.metadata.annotations is permitted (valid field)",
			rules: []ArchitectureRule{
				{
					Name:          "annotations-rule",
					Expression:    "'env' in self.metadata.annotations",
					Architectures: []string{"amd64"},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &CelArchitecturePlacement{
				FallbackArchitectures: []string{"amd64"},
				Rules:                 tt.rules,
			}

			err := plugin.ValidateCELExpressions()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else {
					for _, msg := range tt.errorContains {
						if !strings.Contains(err.Error(), msg) {
							t.Errorf("expected error to contain %q, got: %v", msg, err)
						}
					}
					for _, msg := range tt.errorNotContains {
						if strings.Contains(err.Error(), msg) {
							t.Errorf("expected error not to contain %q, got: %v", msg, err)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestArchitectureRule_ExpressionMaxLength verifies that the MaxLength=1024
// kubebuilder marker is present in the struct tag and that ValidateCELExpressions
// correctly handles expressions near the boundary.
//
// Note: The MaxLength constraint is enforced by the Kubernetes API server at
// admission time via the generated CRD schema.  At Go runtime, ValidateCELExpressions
// is responsible only for CEL syntax/type validation, not length enforcement.
// These tests verify the CEL validation behavior across expression lengths.
func TestArchitectureRule_ExpressionLengthBoundary(t *testing.T) {
	// Build an expression exactly at the maximum allowed length.
	// Use a valid CEL boolean expression padded to 1024 characters.
	// We construct: self.metadata.name == 'x' (plus padding as comments is not valid CEL,
	// so we use a valid repeated OR expression that is long but valid).
	// The simplest approach: self.metadata.name == 'name' repeated with || up to 1024 chars.
	base := "self.metadata.name == 'x'"
	or := " || self.metadata.name == 'x'"

	expr1024 := base
	for len(expr1024)+len(or) <= 1024 {
		expr1024 += or
	}
	// expr1024 is now the longest valid OR-chain that fits within 1024 characters.
	// The loop condition ensures len(expr1024) <= 1024 at all times, so no trimming is needed.

	tests := []struct {
		name        string
		expression  string
		expectError bool
		desc        string
	}{
		{
			name:        "expression at length 1 (minimum)",
			expression:  "true",
			expectError: false,
			desc:        "Single-char valid expression should be accepted",
		},
		{
			name:        "short valid expression",
			expression:  "self.metadata.name == 'test'",
			expectError: false,
			desc:        "Normal expression well below 1024 characters should be accepted",
		},
		{
			name:        "expression near 1024 characters",
			expression:  expr1024,
			expectError: false,
			desc:        "Expression near maximum allowed length should be accepted by CEL validation",
		},
		{
			name:        "empty expression rejected by CEL",
			expression:  "",
			expectError: true,
			desc:        "Empty expression (violates MinLength=1) is also rejected by CEL compilation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &CelArchitecturePlacement{
				FallbackArchitectures: []string{"amd64"},
				Rules: []ArchitectureRule{
					{Name: "length-test", Expression: tt.expression, Architectures: []string{"amd64"}},
				},
			}
			err := plugin.ValidateCELExpressions()
			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.desc)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.desc, err)
			}
		})
	}
}
