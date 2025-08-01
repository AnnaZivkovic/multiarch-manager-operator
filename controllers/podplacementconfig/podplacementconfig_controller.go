/*
Copyright 2023 Red Hat, Inc.

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
	runtime2 "runtime"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl2 "sigs.k8s.io/controller-runtime/pkg/controller"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	multiarchv1beta1 "github.com/openshift/multiarch-tuning-operator/apis/multiarch/v1beta1"
	"github.com/openshift/multiarch-tuning-operator/controllers/podplacement"
	"github.com/openshift/multiarch-tuning-operator/controllers/podplacement/metrics"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

// Reconciler reconciles a PodPlacementConfig object
type Reconciler struct {
	client.Client
	clientSet *kubernetes.Clientset
	Scheme    *runtime.Scheme
	recorder  record.EventRecorder
}

func NewReconciler(client client.Client, clientSet *kubernetes.Clientset, scheme *runtime.Scheme, recorder record.EventRecorder) *Reconciler {
	return &Reconciler{
		Client:    client,
		clientSet: clientSet,
		Scheme:    scheme,
		recorder:  recorder,
	}
}

//+kubebuilder:rbac:groups=multiarch.openshift.io,resources=podplacementconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multiarch.openshift.io,resources=podplacementconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multiarch.openshift.io,resources=podplacementconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PodPlacementConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Lazy initialization of the metrics to support concurrent reconciles
	metrics.InitPodPlacementControllerMetrics()
	now := time.Now()
	defer utils.HistogramObserve(now, metrics.TimeToProcessPod)
	log := ctrllog.FromContext(ctx)

	pod := podplacement.NewPod(&corev1.Pod{}, ctx, r.recorder)

	if err := r.Get(ctx, req.NamespacedName, pod.PodObject()); err != nil {
		log.V(2).Info("Unable to fetch pod", "error", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Pods without the scheduling gate should be ignored.
	if !pod.HasSchedulingGate() {
		log.V(2).Info("Pod does not have the scheduling gate. Ignoring...")
		return ctrl.Result{}, nil
	}
	metrics.ProcessedPodsCtrl.Inc()
	defer utils.HistogramObserve(now, metrics.TimeToProcessGatedPod)
	r.processPod(ctx, pod)
	err := r.Update(ctx, pod.PodObject())
	if err != nil {
		log.Error(err, "Unable to update the pod")
		pod.PublishEvent(corev1.EventTypeWarning, podplacement.ArchitectureAwareSchedulingGateRemovalFailure, podplacement.SchedulingGateRemovalFailureMsg)
		return ctrl.Result{}, err
	}
	if !pod.HasSchedulingGate() {
		// Only publish the event if the scheduling gate has been removed and the pod has been updated successfully.
		pod.PublishEvent(corev1.EventTypeNormal, podplacement.ArchitectureAwareSchedulingGateRemovalSuccess, podplacement.SchedulingGateRemovalSuccessMsg)
		metrics.GatedPodsGauge.Dec()
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) processPod(ctx context.Context, pod *podplacement.Pod) {
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Processing pod")

	// Get all PodPlacementConfig objects in the pod's namespace
	ppcList := &multiarchv1beta1.PodPlacementConfigList{}
	if err := r.List(ctx, ppcList, client.InNamespace(pod.Namespace)); err != nil {
		pod.HandleError(err, "Unable to list PodPlacementConfigs")
		return
	}

	// Sort the configurations by descending priority
	sort.Slice(ppcList.Items, func(i, j int) bool {
		return ppcList.Items[i].Spec.Priority > ppcList.Items[j].Spec.Priority
	})

	podModified := false
	podCopy := pod.DeepCopy()

	// For each namespace-scoped configuration, check selector and apply
	for _, ppc := range ppcList.Items {
		// TODO: idk if this should be here
		// check if plugin is enabled
		if ppc.Spec.Plugins == nil || ppc.Spec.Plugins.NodeAffinityScoring == nil || ppc.Spec.Plugins.NodeAffinityScoring.IsEnabled() {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(ppc.Spec.LabelSelector)
		if err != nil {
			log.Error(err, "Invalid label selector in PodPlacementConfig", "name", ppc.Name)
			continue
		}

		// Check if the pod matches the label selector
		if selector.Matches(labels.Set(podCopy.Labels)) {
			log.Info("Applying namespace-scoped config", "PodPlacementConfig", ppc.Name)
			// Apply the configuration, checking for overlaps
			psdl, err := r.pullSecretDataList(ctx, pod)

			_, err = pod.SetNodeAffinityArchRequirement(psdl)
			pod.HandleError(err, "Unable to set the node affinity requirement for the pod.")
			//if modified {
			//	podModified = true
			//}
		}
	}

	//// 6. Apply the cluster-scoped ClusterPodPlacementConfig configuration
	//clusterConfig := clusterpodplacementconfig.GetClusterPodPlacementConfig()
	//if clusterConfig != nil {
	//	log.Info("Applying cluster-scoped config")
	//	// Apply strong predicates (required affinities)
	//	modified := r.applyRequiredAffinities(podCopy, clusterConfig)
	//	if modified {
	//		podModified = true
	//	}
	//	// Apply preferred affinities, respecting existing settings
	//	modified = r.applyClusterPreferredAffinities(podCopy, clusterConfig)
	//	if modified {
	//		podModified = true
	//	}
	//}

	// 7. Remove the scheduling gate
	newGates := []corev1.PodSchedulingGate{}
	for _, gate := range podCopy.Spec.SchedulingGates {
		if gate.Name != utils.SchedulingGateName {
			newGates = append(newGates, gate)
		}
	}

	// Check if the gate was actually removed to determine if an update is needed
	if len(newGates) < len(podCopy.Spec.SchedulingGates) {
		podCopy.Spec.SchedulingGates = newGates
		podModified = true
	}

	// 8. If any changes were made, update the pod object in the cluster
	if podModified {
		log.Info("Updating pod with new affinities and removing scheduling gate")
		if err := r.Patch(ctx, podCopy, client.MergeFrom(pod)); err != nil {
			log.Error(err, "Failed to update Pod")
			return
		}
	}
}

// pullSecretDataList returns the list of secrets data for the given pod given its imagePullSecrets field
func (r *Reconciler) pullSecretDataList(ctx context.Context, pod *podplacement.Pod) ([][]byte, error) {
	log := ctrllog.FromContext(ctx)
	secretAuths := make([][]byte, 0)
	secretList := pod.GetPodImagePullSecrets()
	for _, pullsecret := range secretList {
		secret, err := r.clientSet.CoreV1().Secrets(pod.Namespace).Get(ctx, pullsecret, metav1.GetOptions{})
		if err != nil {
			log.Error(err, "Error getting secret", "secret", pullsecret)
			continue
		}
		if secretData, err := utils.ExtractAuthFromSecret(secret); err != nil {
			log.Error(err, "Error extracting auth from secret", "secret", pullsecret)
			continue
		} else {
			secretAuths = append(secretAuths, secretData)
		}
	}
	return secretAuths, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// This reconciler is mostly I/O bound due to the pod and node retrievals, so we can increase the number of concurrent
	// reconciles to the number of CPUs * 4.
	maxConcurrentReconciles := runtime2.NumCPU() * 4
	ctrllog.FromContext(context.Background()).Info("Setting up the ENoExecEventReconciler with the manager with max"+
		" concurrent reconciles", "maxConcurrentReconciles", maxConcurrentReconciles)

	return ctrl.NewControllerManagedBy(mgr).
		For(&multiarchv1beta1.ENoExecEvent{}).
		WithOptions(ctrl2.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
		}).
		Complete(r)
}
