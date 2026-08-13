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
	"fmt"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	lru "github.com/hashicorp/golang-lru/v2/simplelru"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/multiarch-tuning-operator/api/common/plugins"
)

// celExpressionCacheSize is the maximum number of compiled CEL programs retained
// in the LRU cache.  Each compiled program is a few KB; 1024 entries caps memory
// at a few MB even with many PPC revisions over the operator lifetime.
const celExpressionCacheSize = 1024

// celEvaluator handles CEL expression compilation, caching, and evaluation
type celEvaluator struct {
	env   *cel.Env
	cache *lru.LRU[string, cel.Program]
	mu    sync.Mutex
}

var (
	// packageEvaluator is a package-level evaluator for expression caching
	packageEvaluator *celEvaluator
	// packageEvaluatorErr holds the initialization error so subsequent calls can
	// surface it rather than silently returning a nil evaluator.
	packageEvaluatorErr error
	// evaluatorOnce ensures the evaluator is initialized only once
	evaluatorOnce sync.Once
)

// getOrCreateEvaluator returns the package-level evaluator, creating it if necessary.
// This enables expression caching across pod evaluations as specified in the enhancement.
// If initialization failed the first time, every subsequent call returns the same error.
func getOrCreateEvaluator() (*celEvaluator, error) {
	evaluatorOnce.Do(func() {
		packageEvaluator, packageEvaluatorErr = newCELEvaluator()
	})
	if packageEvaluatorErr != nil {
		return nil, packageEvaluatorErr
	}
	return packageEvaluator, nil
}

// newCELEvaluator creates a new CEL evaluator with a Pod-aware environment
func newCELEvaluator() (*celEvaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable("self", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	cache, err := lru.NewLRU[string, cel.Program](celExpressionCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL expression LRU cache: %w", err)
	}

	return &celEvaluator{
		env:   env,
		cache: cache,
	}, nil
}

// compile compiles a CEL expression and caches the result.
// simplelru.LRU is not thread-safe; all cache access is serialised through e.mu.
func (e *celEvaluator) compile(expression string) (cel.Program, error) {
	// Check cache first (fast path: hold lock only for the map lookup).
	e.mu.Lock()
	if prog, found := e.cache.Get(expression); found {
		e.mu.Unlock()
		return prog, nil
	}
	e.mu.Unlock()

	// Compile expression (lock-free: env.Compile is stateless and reentrant).
	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL compilation error: %w", issues.Err())
	}

	// Check that the expression returns a boolean
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("CEL expression must return a boolean, got %v", ast.OutputType())
	}

	// Create program
	prog, err := e.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Store the compiled program in the LRU cache (evicts oldest when full).
	e.mu.Lock()
	e.cache.Add(expression, prog)
	e.mu.Unlock()

	return prog, nil
}

// podToMap converts a Pod to a map structure that CEL can evaluate
func podToMap(pod *corev1.Pod) map[string]interface{} {
	if pod == nil {
		return map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":         "",
				"generateName": "",
				"namespace":    "",
				"labels":       map[string]interface{}{},
				"annotations":  map[string]interface{}{},
			},
		}
	}

	labels := make(map[string]interface{})
	for k, v := range pod.Labels {
		labels[k] = v
	}

	annotations := make(map[string]interface{})
	for k, v := range pod.Annotations {
		annotations[k] = v
	}

	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":         pod.Name,
			"generateName": pod.GenerateName,
			"namespace":    pod.Namespace,
			"labels":       labels,
			"annotations":  annotations,
		},
	}
}

// evaluateWithMap evaluates a compiled CEL expression against a pre-built pod map.
// Returns true if the expression matches, false otherwise.
// Errors from compilation or runtime evaluation are returned to the caller;
// it is the caller's responsibility to decide how to handle them (e.g. skip or abort).
func (e *celEvaluator) evaluateWithMap(expression string, podMap map[string]interface{}) (bool, error) {
	prog, err := e.compile(expression)
	if err != nil {
		return false, err
	}

	// Evaluate the expression
	val, _, err := prog.Eval(map[string]interface{}{
		"self": podMap,
	})
	if err != nil {
		// With DynType, accessing a field that is absent in the underlying
		// map produces a "no such key" runtime error rather than a static
		// type error.  Treat these as a non-match (false) rather than a
		// hard error so that expressions like
		//   self.metadata.nonexistent == 'value'
		// gracefully evaluate to false instead of blocking pod admission.
		if strings.Contains(err.Error(), "no such key") {
			return false, nil
		}
		return false, fmt.Errorf("CEL evaluation error: %w", err)
	}

	// Belt-and-suspenders: compile() already rejects non-boolean expressions,
	// but check the runtime type to guard against unexpected cel-go behaviour.
	if val.Type() != types.BoolType {
		return false, fmt.Errorf("CEL expression did not return a boolean: got %v", val.Type())
	}
	return val.Value().(bool), nil
}

// evaluate evaluates a CEL expression against a Pod.
// podToMap is called internally; prefer evaluateWithMap when evaluating multiple
// rules for the same Pod to avoid repeated conversion.
func (e *celEvaluator) evaluate(expression string, pod *corev1.Pod) (bool, error) {
	return e.evaluateWithMap(expression, podToMap(pod))
}

// rulesEvalResult is returned by evaluateRules to distinguish between three
// distinct outcomes that the caller must handle differently:
//
//   - matched=true:  a rule evaluated successfully to true → use its architectures
//   - matched=false, allErrored=false: all rules evaluated to false → use fallback
//   - matched=false, allErrored=true:  every rule failed to compile or evaluate
//     (the PPC's CEL configuration is malformed); the caller decides what to do
type rulesEvalResult struct {
	architectures []string
	ruleName      string
	matched       bool
	// allErrored is true when every rule produced an evaluation error and no
	// rule was successfully evaluated (even to false).  The caller must treat
	// the PPC as invalid and skip it rather than applying fallback.
	allErrored bool
}

// evaluateRules evaluates CEL rules in order and returns the first matching rule's architectures.
// Rules that fail to compile or evaluate at runtime are skipped; evaluation continues with
// the next rule. This soft-failure model keeps pod admission alive even when a single
// expression has an unexpected runtime fault.
//
// podToMap is called once per Pod and the resulting map is reused across all rules.
//
// The returned rulesEvalResult.allErrored distinguishes "all rules errored" from
// "all rules evaluated to false", which is critical for the caller to decide
// whether to apply fallback or skip the PPC entirely.
func (e *celEvaluator) evaluateRules(rules []plugins.ArchitectureRule, pod *corev1.Pod) rulesEvalResult {
	// Convert pod to map once — reused for every rule in this PPC.
	podMap := podToMap(pod)

	atLeastOneEvaluated := false
	for _, rule := range rules {
		matched, err := e.evaluateWithMap(rule.Expression, podMap)
		if err != nil {
			// This rule failed to compile or had a runtime error; skip it.
			// Continue to see if subsequent rules are valid.
			continue
		}
		// This rule was successfully evaluated (result is true or false).
		atLeastOneEvaluated = true
		if matched {
			// First match wins; remaining rules are not evaluated.
			return rulesEvalResult{
				architectures: rule.Architectures,
				ruleName:      rule.Name,
				matched:       true,
				allErrored:    false,
			}
		}
	}

	if len(rules) > 0 && !atLeastOneEvaluated {
		// Every rule produced an error; the PPC configuration is entirely
		// malformed for this pod.  Signal to the caller to skip this PPC.
		return rulesEvalResult{allErrored: true}
	}

	// No rules matched (some may have errored and been skipped); caller should
	// apply fallback architectures.
	return rulesEvalResult{matched: false, allErrored: false}
}

// evaluateResult represents the result of CEL rule evaluation
type evaluateResult struct {
	architectures []string
	ruleName      string
	matched       bool
	// allRulesErrored is true when every rule in the PPC failed to compile or
	// evaluate.  Both the webhook and the controller skip the malformed PPC
	// (return false) so that lower-priority PPCs can still claim the pod.
	// The fallback architectures of the malformed PPC are never applied.
	allRulesErrored bool
}

// evaluateCELArchitecturePlacement evaluates the celArchitecturePlacement plugin rules.
// Returns the architectures to apply and whether a rule matched, or an error when the
// evaluator itself cannot be initialized.
//
// When all rules in the PPC are malformed (compile / evaluation errors), returns a
// result with allRulesErrored=true.  Both the webhook and the controller treat this
// as a skip: the malformed PPC is not applied and lower-priority PPCs can still
// claim the pod.  Uses a package-level evaluator for expression caching across pod
// evaluations.
func evaluateCELArchitecturePlacement(rules []plugins.ArchitectureRule, fallbackArchitectures []string, pod *corev1.Pod) (*evaluateResult, error) {
	if rules == nil && fallbackArchitectures == nil {
		return nil, fmt.Errorf("both rules and fallbackArchitectures are nil")
	}

	// Get or create the package-level evaluator for expression caching
	evaluator, err := getOrCreateEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to get CEL evaluator: %w", err)
	}

	// Evaluate rules in order — podToMap is called once inside evaluateRules.
	rr := evaluator.evaluateRules(rules, pod)

	if rr.allErrored {
		// Every rule in this PPC failed to compile or evaluate.
		// Return fallback architectures and signal the condition via allRulesErrored=true
		// so callers can decide how to handle it without blocking pod admission.
		return &evaluateResult{
			architectures:   fallbackArchitectures,
			matched:         false,
			allRulesErrored: true,
		}, nil
	}

	if rr.matched {
		return &evaluateResult{
			architectures: rr.architectures,
			ruleName:      rr.ruleName,
			matched:       true,
		}, nil
	}

	// No rules matched (and at least one was validly evaluated, or there are no
	// rules at all) — apply fallback architectures as specified by the enhancement.
	return &evaluateResult{
		architectures: fallbackArchitectures,
		ruleName:      "",
		matched:       false,
	}, nil
}

// validateCELExpression validates a CEL expression without evaluating it
// This can be used for validation at admission time
func validateCELExpression(expression string) error {
	evaluator, err := newCELEvaluator()
	if err != nil {
		return err
	}

	_, err = evaluator.compile(expression)
	return err
}
