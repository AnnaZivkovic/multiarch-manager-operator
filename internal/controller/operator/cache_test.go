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
	"fmt"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

func TestCacheByObjectScopesOnlyNetworkPolicy(t *testing.T) {
	byObject := CacheByObject()
	if len(byObject) != 1 {
		t.Fatalf("CacheByObject should scope exactly 1 type (NetworkPolicy), got %d", len(byObject))
	}
	cfg, ok := lookupByType(byObject, &networkingv1.NetworkPolicy{})
	if !ok {
		t.Fatal("CacheByObject missing *networkingv1.NetworkPolicy")
	}
	assertOperatorLocalCache(t, &networkingv1.NetworkPolicy{}, cfg)
}

func TestAddMonitoringCache(t *testing.T) {
	byObject := CacheByObject()
	AddMonitoringCache(byObject)
	for _, obj := range []client.Object{
		&monitoringv1.ServiceMonitor{},
		&monitoringv1.PrometheusRule{},
	} {
		cfg, ok := lookupByType(byObject, obj)
		if !ok {
			t.Fatalf("AddMonitoringCache missing %T", obj)
		}
		assertOperatorLocalCache(t, obj, cfg)
	}
}

func assertOperatorLocalCache(t *testing.T, obj client.Object, cfg cache.ByObject) {
	t.Helper()
	if len(cfg.Namespaces) != 1 {
		t.Fatalf("%T Namespaces: got %d want 1", obj, len(cfg.Namespaces))
	}
	if _, ok := cfg.Namespaces[utils.Namespace()]; !ok {
		t.Fatalf("%T Namespaces: got %v want %q", obj, cfg.Namespaces, utils.Namespace())
	}
}

func lookupByType(byObject map[client.Object]cache.ByObject, want client.Object) (cache.ByObject, bool) {
	wantName := fmt.Sprintf("%T", want)
	for obj, cfg := range byObject {
		if fmt.Sprintf("%T", obj) == wantName {
			return cfg, true
		}
	}
	return cache.ByObject{}, false
}
