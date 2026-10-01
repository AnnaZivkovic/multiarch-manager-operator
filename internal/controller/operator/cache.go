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
	networkingv1 "k8s.io/api/networking/v1"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

// CacheByObject scopes the NetworkPolicy informer to utils.Namespace() so it
// matches the namespace-scoped networkpolicy-role. All other types
// (Deployments, Services, etc.) remain on the ClusterRole and use
// cluster-wide informers.
func CacheByObject() map[client.Object]cache.ByObject {
	return map[client.Object]cache.ByObject{
		&networkingv1.NetworkPolicy{}: {
			Namespaces: map[string]cache.Config{
				utils.Namespace(): {},
			},
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
