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
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

const managerNetworkPolicyRequeue = 5 * time.Second

// ManagerNetworkPolicyReconciler creates and heals the manager NetworkPolicy
// owned by the manager Deployment. The policy is not shipped in the OLM
// bundle (olm.allowed_resource_kinds).
type ManagerNetworkPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=create;delete;get;list;patch;update;watch

func (r *ManagerNetworkPolicyReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      utils.OperatorName + "-controller-manager",
		Namespace: utils.Namespace(),
	}, deployment)
	if apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: managerNetworkPolicyRequeue}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	desired := buildNetworkPolicyManager()
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, policy, func() error {
		if err := ctrl.SetControllerReference(deployment, policy, r.Scheme); err != nil {
			return err
		}
		policy.Labels = desired.Labels
		policy.Annotations = desired.Annotations
		policy.Spec = desired.Spec
		return nil
	})
	return ctrl.Result{}, err
}

func (r *ManagerNetworkPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	managerName := utils.OperatorName + "-controller-manager"
	managerNamespace := utils.Namespace()
	return ctrl.NewControllerManagedBy(mgr).
		Named("manager-network-policy").
		For(&appsv1.Deployment{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetName() == managerName && obj.GetNamespace() == managerNamespace
		}))).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}
