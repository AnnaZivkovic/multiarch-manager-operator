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
	rbacv1 "k8s.io/api/rbac/v1"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	multiarchv1beta1 "github.com/openshift/multiarch-tuning-operator/api/v1beta1"
	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

// CacheByObject scopes operator-local namespaced types to utils.Namespace() so
// the informers match the namespaced Role. Do not add PodPlacementConfig (the
// operator lists it in every namespace) or Pod (deprovision lists pending pods
// cluster-wide via ClientSet). Do not set DefaultNamespaces on the operator
// manager: that would also restrict PodPlacementConfig.
func CacheByObject() map[client.Object]cache.ByObject {
	localNamespaces := map[string]cache.Config{
		utils.Namespace(): {},
	}
	return map[client.Object]cache.ByObject{
		&appsv1.Deployment{}: {
			Namespaces: localNamespaces,
		},
		&appsv1.DaemonSet{}: {
			Namespaces: localNamespaces,
		},
		&corev1.Service{}: {
			Namespaces: localNamespaces,
		},
		&corev1.ServiceAccount{}: {
			Namespaces: localNamespaces,
		},
		&networkingv1.NetworkPolicy{}: {
			Namespaces: localNamespaces,
		},
		&rbacv1.Role{}: {
			Namespaces: localNamespaces,
		},
		&rbacv1.RoleBinding{}: {
			Namespaces: localNamespaces,
		},
		&multiarchv1beta1.ENoExecEvent{}: {
			Namespaces: localNamespaces,
		},
	}
}

// AddMonitoringCache scopes ServiceMonitor and PrometheusRule informers to the
// operator namespace. Call this only when those CRDs are installed; otherwise
// manager.Start fails looking up the types.
func AddMonitoringCache(byObject map[client.Object]cache.ByObject) {
	localNamespaces := map[string]cache.Config{
		utils.Namespace(): {},
	}
	byObject[&monitoringv1.ServiceMonitor{}] = cache.ByObject{
		Namespaces: localNamespaces,
	}
	byObject[&monitoringv1.PrometheusRule{}] = cache.ByObject{
		Namespaces: localNamespaces,
	}
}
