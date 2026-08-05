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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	testingutils "github.com/openshift/multiarch-tuning-operator/pkg/testing/framework"
)

// newEphemeralTestNamespace creates a uniquely-named Namespace suitable for a
// single Ginkgo It block, registers its deletion with DeferCleanup, and returns
// the created Namespace object.
//
// Usage (inside a BeforeEach):
//
//	var ns *corev1.Namespace
//	BeforeEach(func() { ns = newEphemeralTestNamespace() })
//
// The DeferCleanup registered here fires after each It regardless of whether
// the spec passed or failed, ensuring no leftover resources between parallel workers.
func newEphemeralTestNamespace() *corev1.Namespace {
	ns := testingutils.NewEphemeralNamespace("cel-")
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	DeferCleanup(func() {
		// Best-effort: ignore NotFound in case the spec itself deleted the namespace.
		_ = k8sClient.Delete(ctx, ns)
	})
	return ns
}
