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

package podplacementconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/openshift/multiarch-tuning-operator/pkg/e2e"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"

	v1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap/zapcore"

	testingutils "github.com/openshift/multiarch-tuning-operator/pkg/testing/framework"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	stopMgr   context.CancelFunc
	testEnv   *envtest.Environment
	ctx       context.Context
	suiteLog  = ctrl.Log.WithName("setup")
)

func init() {
	e2e.CommonInit()
}

func TestOperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Integration Suite", Label("integration", "operator"))
}

var _ = BeforeAll

var _ = SynchronizedBeforeSuite(func() []byte {
	suiteLog = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.Level(-5)))
	ctx = context.TODO()
	logf.SetLogger(suiteLog)
	SetDefaultEventuallyPollingInterval(5 * time.Millisecond)
	SetDefaultEventuallyTimeout(5 * time.Second)
	startTestEnv()
	testingutils.EnsureNamespaces(ctx, k8sClient, testNamespace)
	runManager()
	kc := testingutils.FromEnvTestConfig(cfg)
	data, err := json.Marshal(kc)
	Expect(err).NotTo(HaveOccurred(), "failed to marshal sharedData")
	return data
}, func(data []byte) {
	suiteLog = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.Level(-5)))
	ctx = context.TODO()
	logf.SetLogger(suiteLog)
	SetDefaultEventuallyPollingInterval(5 * time.Millisecond)
	SetDefaultEventuallyTimeout(5 * time.Second)
	var err error
	var kc api.Config
	err = json.Unmarshal(data, &kc)
	Expect(err).NotTo(HaveOccurred(), "failed to unmarshal sharedData")
	// Sync test cluster environment
	ocg := clientcmd.NewDefaultClientConfig(kc, &clientcmd.ConfigOverrides{})
	cfg, err = ocg.ClientConfig()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	By("tearing down the test environment")
	stopMgr()
	// wait for the manager to stop. FIXME: this is a hack, not sure what is the right way to do it.
	time.Sleep(5 * time.Second)
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func startTestEnv() {
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			ValidatingWebhooks: []*v1.ValidatingWebhookConfiguration{getPodPlacementConfigValidatingWebHook()},
		},
	}
	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
	//+kubebuilder:scaffold:scheme
	crdPath := filepath.Join("..", "..", "config", "crd")
	testingutils.ApplyCRDs(crdPath, k8sClient, ctx)
}

func runManager() {
	By("Creating the manager")
	webhookServer := webhook.NewServer(webhook.Options{
		Port:    testEnv.WebhookInstallOptions.LocalServingPort,
		Host:    testEnv.WebhookInstallOptions.LocalServingHost,
		CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
	})
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		HealthProbeBindAddress: ":4980",
		Logger:                 suiteLog,
		WebhookServer:          webhookServer,
	})
	Expect(err).NotTo(HaveOccurred())

	suiteLog.Info("Manager created")

	By("Setting up PodPlacement controller")
	mgr.GetWebhookServer().Register("/validate-multiarch-openshift-io-v1beta1-podplacementconfig", &webhook.Admission{
		Handler: NewPodPlacementConfigWebhook(mgr.GetAPIReader(), mgr.GetScheme())})

	err = mgr.AddReadyzCheck("readyz", healthz.Ping)
	Expect(err).NotTo(HaveOccurred())

	By("Starting the manager")
	go func() {
		var mgrCtx context.Context
		mgrCtx, stopMgr = context.WithCancel(ctx)
		err = mgr.Start(mgrCtx)
		Expect(err).NotTo(HaveOccurred())
	}()

	By("Waiting for the manager to be ready...")
	Eventually(func(g Gomega) {
		resp, err := http.Get("http://127.0.0.1:4980/readyz")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	}).MustPassRepeatedly(3).Should(
		Succeed(), "manager is not ready yet")
	suiteLog.Info("Manager is ready")
}

func getPodPlacementConfigValidatingWebHook() *v1.ValidatingWebhookConfiguration {
	return &v1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "validating-webhook-configuration",
		},
		Webhooks: []v1.ValidatingWebhook{
			{
				Name:                    "validate-podplacementconfig.multiarch.openshift.io",
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig: v1.WebhookClientConfig{
					Service: &v1.ServiceReference{
						Name:      "webhook-service",
						Namespace: "system",
						Path:      utils.NewPtr("/validate-multiarch-openshift-io-v1beta1-podplacementconfig"),
					},
				},
				FailurePolicy: utils.NewPtr(v1.Fail),
				SideEffects:   utils.NewPtr(v1.SideEffectClassNone),
				Rules: []v1.RuleWithOperations{
					{
						Operations: []v1.OperationType{
							v1.Create,
							v1.Update,
							v1.Delete,
						},
						Rule: v1.Rule{
							APIGroups:   []string{"multiarch.openshift.io"},
							APIVersions: []string{"v1beta1"},
							Resources:   []string{"podplacementconfigs"},
						},
					},
				},
			},
		},
	}
}
