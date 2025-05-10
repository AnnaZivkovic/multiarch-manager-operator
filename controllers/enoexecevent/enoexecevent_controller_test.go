package enoexecevent

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/multiarch-tuning-operator/apis/multiarch/v1beta1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/multiarch-tuning-operator/pkg/testing/framework"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"

	"github.com/openshift/multiarch-tuning-operator/pkg/testing/builder"
)

var _ = Describe("Controllers/ENoExecEvent/ENoExecEventReconciler", Serial, Ordered, func() {
	When("The ENoExecEvent", func() {
		Context("is handling the lifecycle of the operand", func() {
			It("should create a ENoExecEvent CR", func() {
				By("Creating the ENoExecEvent")
				enee := builder.NewENoExecEvent().WithName("test").WithNamespace(utils.Namespace()).Build()
				err := k8sClient.Create(ctx, enee)
				Expect(err).NotTo(HaveOccurred(), "failed to create ENoExecEvent", err)
				By("Deleting the ENoExecEvent")
				err = k8sClient.Delete(ctx, enee)
				Expect(err).NotTo(HaveOccurred(), "failed to delete ENoExecEvent", err)
				Eventually(framework.ValidateDeletion(k8sClient, ctx)).Should(Succeed(), "the ENoExecEvent should be deleted")
			})
			It("should create a ENoExecEvent CR with set fields", func() {
				By("Creating the ENoExecEvent")
				enee := builder.NewENoExecEvent().WithName("test-enee").WithNamespace(utils.Namespace()).
					WithNodeName("foo").WithPodName("bar").WithPodNamespace("foo").
					WithContainerID("bar").WithCommand("foo").Build()
				err := k8sClient.Create(ctx, enee)
				Expect(err).NotTo(HaveOccurred(), "failed to create ENoExecEvent", err)
				Eventually(func(g Gomega) {
					// Get enee from the API server
					By("Ensure no ClusterPodPlacementConfig exists")
					enee = &v1beta1.ENoExecEvent{}
					err := k8sClient.Get(ctx, crclient.ObjectKey{
						Name: "test-enee",
					}, enee)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get enee", err)
					g.Expect(enee.Status.NodeName).To(Equal("foo"))
					g.Expect(enee.Status.PodName).To(Equal("bar"))
					g.Expect(enee.Status.PodNamespace).To(Equal("foo"))
					g.Expect(enee.Status.ContainerID).To(Equal("bar"))
					g.Expect(enee.Status.Command).To(Equal("foo"))
				}).Should(Succeed(), "failed to get enee")
				By("Deleting the ENoExecEvent")
				err = k8sClient.Delete(ctx, enee)
				Expect(err).NotTo(HaveOccurred(), "failed to delete ENoExecEvent", err)
				Eventually(framework.ValidateDeletion(k8sClient, ctx)).Should(Succeed(), "the ENoExecEvent should be deleted")
			})
		})
	})
})
