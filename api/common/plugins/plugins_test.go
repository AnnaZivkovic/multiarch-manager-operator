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
				} else if tt.errorContains != "" && err.Error() != tt.errorContains {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorContains, err.Error())
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
		name          string
		rules         []ArchitectureRule
		expectError   bool
		errorContains string
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
			errorContains: `"bad-rule"`,
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
			errorContains: `"non-bool"`,
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
			errorContains: `"bad-rule"`,
		},
		// assertMetadataOnly: self.spec.* and self.status.* must be rejected
		{
			name: "self.spec reference is rejected (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "spec-rule",
					Expression:    "self.spec.nodeName == 'worker-1'",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: "disallowed field",
		},
		{
			name: "self.status reference is rejected (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "status-rule",
					Expression:    "self.status.phase == 'Running'",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: "disallowed field",
		},
		{
			name: "self.spec.containers reference is rejected (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "containers-rule",
					Expression:    "size(self.spec.containers) > 0",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: "disallowed field",
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
			name: "has(self.spec.nodeName) is rejected (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "has-spec-rule",
					Expression:    "has(self.spec.nodeName)",
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: "disallowed field",
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
			expectError:   true,
			errorContains: `"spec-rule"`,
		},
		// Regression: comprehension (exists) traverses self.spec.* — must be rejected
		{
			name: "comprehension over self.spec.containers is rejected (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "exists-rule",
					Expression:    `self.spec.containers.exists(c, c.name == "nginx")`,
					Architectures: []string{"amd64"},
				},
			},
			expectError:   true,
			errorContains: "disallowed field",
		},
		// Regression: constant expression has no Select nodes — assertMetadataOnly must not reject it
		{
			name: "constant expression true is permitted (assertMetadataOnly)",
			rules: []ArchitectureRule{
				{
					Name:          "const-rule",
					Expression:    "true",
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
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}
