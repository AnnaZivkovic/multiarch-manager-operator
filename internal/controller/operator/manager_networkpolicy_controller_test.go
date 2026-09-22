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

package operator

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/multiarch-tuning-operator/pkg/testing/builder"
	"github.com/openshift/multiarch-tuning-operator/pkg/testing/framework"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

func managerDeployment() *appsv1.Deployment {
	return builder.NewDeployment().
		WithName(utils.OperatorName + "-controller-manager").
		WithNamespace(utils.Namespace()).
		WithSelectorAndPodLabels(map[string]string{"control-plane": "controller-manager"}).
		WithPodSpec(corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "manager",
				Image: "pause",
			}},
		}).
		Build()
}

var _ = Describe("The ManagerNetworkPolicyReconciler", func() {
	BeforeEach(func() {
		By("Creating the manager Deployment")
		Expect(k8sClient.Create(ctx, managerDeployment())).To(Succeed())
		Eventually(framework.VerifyManagerNetworkPolicy(ctx, k8sClient)).Should(Succeed(),
			"the manager NetworkPolicy should be created for the manager Deployment")
	})
	AfterEach(func() {
		By("Deleting the manager Deployment and NetworkPolicy")
		// Delete both explicitly — don't rely on GC cascade in envtest
		Expect(crclient.IgnoreNotFound(
			k8sClient.Delete(ctx, managerDeployment()))).To(Succeed())
		Expect(crclient.IgnoreNotFound(
			k8sClient.Delete(ctx, &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.ManagerNetworkPolicyName,
					Namespace: utils.Namespace(),
				},
			}))).To(Succeed())

		// Wait for BOTH to be fully gone before returning
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, crclient.ObjectKey{
				Name:      utils.OperatorName + "-controller-manager",
				Namespace: utils.Namespace(),
			}, &appsv1.Deployment{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"deployment should be gone")

			err = k8sClient.Get(ctx, crclient.ObjectKey{
				Name:      utils.ManagerNetworkPolicyName,
				Namespace: utils.Namespace(),
			}, &networkingv1.NetworkPolicy{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"network policy should be gone")
		}).Should(Succeed())
	})
	It("should reconcile the manager NetworkPolicy if deleted", func() {
		By("Deleting " + utils.ManagerNetworkPolicyName)
		err := k8sClient.Delete(ctx, builder.NewNetworkPolicy().
			WithName(utils.ManagerNetworkPolicyName).
			WithNamespace(utils.Namespace()).
			Build())
		Expect(err).NotTo(HaveOccurred(), "failed to delete NetworkPolicy "+utils.ManagerNetworkPolicyName, err)
		Eventually(framework.VerifyManagerNetworkPolicy(ctx, k8sClient)).Should(Succeed(),
			"the manager NetworkPolicy should be recreated")
	})
	It("should reconcile the manager NetworkPolicy if changed", func() {
		np := &networkingv1.NetworkPolicy{}
		err := k8sClient.Get(ctx, crclient.ObjectKey{
			Name:      utils.ManagerNetworkPolicyName,
			Namespace: utils.Namespace(),
		}, np)
		Expect(err).NotTo(HaveOccurred(), "failed to get NetworkPolicy "+utils.ManagerNetworkPolicyName, err)
		By("clearing the NetworkPolicy egress rules")
		np.Spec.Egress = nil
		err = k8sClient.Update(ctx, np)
		Expect(err).NotTo(HaveOccurred(), "failed to update NetworkPolicy "+utils.ManagerNetworkPolicyName, err)
		Eventually(framework.VerifyManagerNetworkPolicy(ctx, k8sClient)).Should(Succeed(),
			"the manager NetworkPolicy should be restored")
	})
})
