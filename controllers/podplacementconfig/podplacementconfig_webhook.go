package podplacementconfig

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openshift/multiarch-tuning-operator/apis/multiarch/common"
	multiarchv1beta1 "github.com/openshift/multiarch-tuning-operator/apis/multiarch/v1beta1"
	"github.com/openshift/multiarch-tuning-operator/controllers/podplacement/metrics"
)

// +kubebuilder:webhook:path=/validate-multiarch-openshift-io-v1beta1-podplacementconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=multiarch.openshift.io,resources=podplacementconfigs,verbs=create;update;delete,versions=v1beta1,name=validate-podplacementconfig.multiarch.openshift.io,admissionReviewVersions=v1

type PodPlacementConfigWebhook struct {
	apiReader client.Reader
	decoder   admission.Decoder
	once      sync.Once
	scheme    *runtime.Scheme
}

func (w *PodPlacementConfigWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	w.once.Do(func() {
		w.decoder = admission.NewDecoder(w.scheme)
	})

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		// Decode new object
		newPPC := &multiarchv1beta1.PodPlacementConfig{}
		if err := w.decoder.Decode(req, newPPC); err != nil {
			return admission.Errored(http.StatusBadRequest,
				fmt.Errorf("failed to decode new PodPlacementConfig: %w", err))
		}

		// Check for duplicate architectures in NodeAffinityScoring
		if newPPC.PluginsEnabled(common.NodeAffinityScoringPluginName) {
			if ok, err := newPPC.Spec.Plugins.NodeAffinityScoring.ValidateArchitecturesSet(); !ok {
				return admission.Denied(err.Error())
			}
		}

		// List existing PodPlacementConfigs in the same namespace
		existingPPCs := &multiarchv1beta1.PodPlacementConfigList{}
		if err := w.apiReader.List(ctx, existingPPCs, client.InNamespace(req.Namespace)); err != nil {
			return admission.Errored(http.StatusInternalServerError,
				fmt.Errorf("failed to list existing PodPlacementConfigs in namespace %q: %w", req.Namespace, err))
		}

		if req.Operation == admissionv1.Create {
			if ok, err := newPPC.ValidatePriorityNew(existingPPCs); !ok {
				return admission.Denied(err.Error())
			}
		} else { // admissionv1.Update
			oldPPC := &multiarchv1beta1.PodPlacementConfig{}
			if err := w.decoder.DecodeRaw(req.OldObject, oldPPC); err != nil {
				return admission.Errored(http.StatusBadRequest,
					fmt.Errorf("failed to decode old PodPlacementConfig: %w", err))
			}
			if ok, err := newPPC.ValidatePriorityUpdate(oldPPC, existingPPCs); !ok {
				return admission.Denied(err.Error())
			}
		}

		return admission.Allowed("valid PodPlacementConfig")

	default:
		return admission.Allowed("operation not explicitly handled")
	}
}

func NewPodPlacementConfigWebhook(apiReader client.Reader, scheme *runtime.Scheme) *PodPlacementConfigWebhook {
	a := &PodPlacementConfigWebhook{
		apiReader: apiReader,
		scheme:    scheme,
	}
	metrics.InitWebhookMetrics()
	return a
}
