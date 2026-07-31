package utils

import (
	"context"
	"fmt"
	"sync"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcehelper"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

var resourceCache resourceapply.ResourceCache

// DeleterInterface abstracts the Delete method of a typed and namespaced client got from a clientset
type DeleterInterface interface {
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

// Wrapper struct that holds the dynamic client for the specific resource
type dynamicDeleter struct {
	dynamicClient dynamic.ResourceInterface
}

// Delete implements the DeleterInterface's Delete method for the dynamicDeleter struct
func (d *dynamicDeleter) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	// Call the dynamic client's Delete method and ignore the subresources variadic parameter
	return d.dynamicClient.Delete(ctx, name, opts)
}

func NewDynamicDeleter(dynamicClient dynamic.ResourceInterface) DeleterInterface {
	return &dynamicDeleter{dynamicClient: dynamicClient}
}

type ToDeleteRef struct {
	NamespacedTypedClient DeleterInterface
	ObjName               string
}

func init() {
	resourceCache = resourceapply.NewResourceCache()
}

// ApplyResource applies the given object to the cluster. It returns the object as it is in the cluster, a boolean
// indicating if the object was created or updated and an error if any.
// TODO[integration-tests]: integration tests for this function in a suite dedicated to this package
func ApplyResource(ctx context.Context, clientSet *kubernetes.Clientset, client *dynamic.DynamicClient, recorder events.Recorder,
	obj client.Object) (client.Object, bool, error) {
	switch t := obj.(type) {
	case *appsv1.Deployment:
		return applyDeployment(ctx, clientSet.AppsV1(), recorder, t)
	case *appsv1.DaemonSet:
		return applyDaemonSet(ctx, clientSet.AppsV1(), recorder, t)
	case *corev1.Service:
		return applyService(ctx, clientSet.CoreV1(), recorder, t)
	case *admissionv1.MutatingWebhookConfiguration:
		return resourceapply.ApplyMutatingWebhookConfigurationImproved(ctx, clientSet.AdmissionregistrationV1(),
			recorder, t, resourceCache)
	case *rbacv1.Role:
		return resourceapply.ApplyRole(ctx, clientSet.RbacV1(), recorder, t)
	case *rbacv1.RoleBinding:
		return resourceapply.ApplyRoleBinding(ctx, clientSet.RbacV1(), recorder, t)
	case *corev1.ServiceAccount:
		return resourceapply.ApplyServiceAccount(ctx, clientSet.CoreV1(), recorder, t)
	case *rbacv1.ClusterRole:
		return resourceapply.ApplyClusterRole(ctx, clientSet.RbacV1(), recorder, t)
	case *rbacv1.ClusterRoleBinding:
		return resourceapply.ApplyClusterRoleBinding(ctx, clientSet.RbacV1(), recorder, t)
	case *monitoringv1.ServiceMonitor:
		objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(t)
		if err != nil {
			return nil, false, err
		}
		return resourceapply.ApplyServiceMonitor(ctx, client, recorder, &unstructured.Unstructured{
			Object: objMap,
		})
	case *monitoringv1.PrometheusRule:
		objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(t)
		if err != nil {
			return nil, false, err
		}
		return resourceapply.ApplyPrometheusRule(ctx, client, recorder, &unstructured.Unstructured{
			Object: objMap,
		})
	default:
		return nil, false, fmt.Errorf("unhandled type %T", obj)
	}
}

// ApplyResources applies the given objects to the cluster. It returns an error if any.
// TODO[integration-tests]: integration tests for this function in a suite dedicated to this package
func ApplyResources(ctx context.Context, clientSet *kubernetes.Clientset, client *dynamic.DynamicClient, recorder events.Recorder,
	objs []client.Object) error {
	log := ctrllog.FromContext(ctx)
	var errs []error
	for _, obj := range objs {
		log.Info("Applying object", "name", obj.GetName(), "type", fmt.Sprintf("%T", obj))
		_, _, err := ApplyResource(ctx, clientSet, client, recorder, obj)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// DeleteResource deletes the given object from the cluster. It returns an error if any.
// TODO[integration-tests]: integration tests for this function in a suite dedicated to this package
func DeleteResource(ctx context.Context, namespacedTypedClient DeleterInterface, objName string) error {
	return client.IgnoreNotFound(namespacedTypedClient.Delete(ctx, objName, metav1.DeleteOptions{}))
}

// DeleteResources deletes the given objects from the cluster. It returns an error if any.
// TODO[integration-tests]: integration tests for this function in a suite dedicated to this package
func DeleteResources(ctx context.Context, toDeleteRefs []ToDeleteRef) error {
	errs := make([]error, 0)
	log := ctrllog.FromContext(ctx)
	for _, toDeleteRef := range toDeleteRefs {
		log.Info("Deleting object", "name", toDeleteRef.ObjName, "type",
			fmt.Sprintf("%T", toDeleteRef.NamespacedTypedClient))
		if err := DeleteResource(ctx, toDeleteRef.NamespacedTypedClient, toDeleteRef.ObjName); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// generations tracks the last-applied generation for each Deployment/DaemonSet using
// the standard OpenShift operatorv1.GenerationStatus type and library-go helpers.
// On pod restart the slice is empty, so the first reconcile always applies
// (ExpectedDeploymentGeneration returns -1 when no entry is found).
var (
	generationsMu sync.Mutex
	generations   []operatorv1.GenerationStatus
)

// ResetGenerations clears the generation tracking state. Intended for test isolation.
func ResetGenerations() {
	generationsMu.Lock()
	defer generationsMu.Unlock()
	generations = nil
}

// applyDeployment wraps library-go's ApplyDeployment with generation tracking so that
// the spec-hash skip path works correctly. Without tracking, passing expectedGeneration=0
// disables the skip path entirely, causing an Update on every reconcile.
func applyDeployment(ctx context.Context, client appsv1client.DeploymentsGetter, recorder events.Recorder,
	required *appsv1.Deployment) (*appsv1.Deployment, bool, error) {
	generationsMu.Lock()
	defer generationsMu.Unlock()
	expectedGen := resourcemerge.ExpectedDeploymentGeneration(required, generations)
	result, modified, err := resourceapply.ApplyDeployment(ctx, client, recorder, required, expectedGen)
	if err == nil && result != nil {
		resourcemerge.SetDeploymentGeneration(&generations, result)
	}
	return result, modified, err
}

// applyDaemonSet wraps library-go's ApplyDaemonSet with generation tracking.
func applyDaemonSet(ctx context.Context, client appsv1client.DaemonSetsGetter, recorder events.Recorder,
	required *appsv1.DaemonSet) (*appsv1.DaemonSet, bool, error) {
	generationsMu.Lock()
	defer generationsMu.Unlock()
	expectedGen := resourcemerge.ExpectedDaemonSetGeneration(required, generations)
	result, modified, err := resourceapply.ApplyDaemonSet(ctx, client, recorder, required, expectedGen)
	if err == nil && result != nil {
		resourcemerge.SetDaemonSetGeneration(&generations, result)
	}
	return result, modified, err
}

// applyService compares the fields the operator manages (ports, selector, type) against the
// existing Service and only issues an Update when they differ. Server-defaulted fields
// (clusterIP, sessionAffinity, ipFamilies, etc.) are ignored to avoid unnecessary updates
// that would trigger Owns() watch events and feed a reconciliation storm.
func applyService(ctx context.Context, client v1.ServicesGetter, recorder events.Recorder,
	requiredOriginal *corev1.Service) (*corev1.Service, bool, error) {
	required := requiredOriginal.DeepCopy()
	existing, err := client.Services(required.Namespace).Get(ctx, required.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		requiredCopy := required.DeepCopy()
		actual, err := client.Services(requiredCopy.Namespace).
			Create(ctx, resourcemerge.WithCleanLabelsAndAnnotations(requiredCopy).(*corev1.Service),
				metav1.CreateOptions{})
		gvk := resourcehelper.GuessObjectGroupVersionKind(requiredCopy)
		if err == nil {
			recorder.Eventf(fmt.Sprintf("%sCreated", gvk.Kind), "Created %s because it was missing",
				resourcehelper.FormatResourceForCLIWithNamespace(requiredCopy))
		} else {
			recorder.Warningf(fmt.Sprintf("%sCreateFailed", gvk.Kind), "Failed to create %s: %v",
				resourcehelper.FormatResourceForCLIWithNamespace(requiredCopy), err)
		}
		return actual, true, err
	}
	if err != nil {
		return nil, false, err
	}

	if serviceSpecMatchesRequired(existing.Spec, required.Spec) {
		return existing, false, nil
	}

	existingCopy := existing.DeepCopy()
	existingCopy.Spec = required.Spec

	actual, err := client.Services(required.Namespace).Update(ctx, existingCopy, metav1.UpdateOptions{})
	gvk := resourcehelper.GuessObjectGroupVersionKind(required)
	if err != nil {
		recorder.Warningf(fmt.Sprintf("%sUpdateFailed", gvk.Kind), "Failed to update %s: %v",
			resourcehelper.FormatResourceForCLIWithNamespace(required), err)
	} else {
		recorder.Eventf(fmt.Sprintf("%sUpdated", gvk.Kind), "Updated %s because it changed",
			resourcehelper.FormatResourceForCLIWithNamespace(required))
	}
	return actual, true, err
}

// serviceSpecMatchesRequired compares only the fields the operator manages:
// ports (name, protocol, port, targetPort), selector, and type.
func serviceSpecMatchesRequired(existing, required corev1.ServiceSpec) bool {
	if !equality.Semantic.DeepEqual(existing.Selector, required.Selector) {
		return false
	}
	existingType := existing.Type
	if len(existingType) == 0 {
		existingType = corev1.ServiceTypeClusterIP
	}
	requiredType := required.Type
	if len(requiredType) == 0 {
		requiredType = corev1.ServiceTypeClusterIP
	}
	if existingType != requiredType {
		return false
	}
	return servicePortsMatch(existing.Ports, required.Ports)
}

// servicePortsMatch compares the managed port fields, ignoring server-defaulted
// fields like NodePort. Ports are compared positionally (buildService always
// produces the same ordered list).
func servicePortsMatch(existing, required []corev1.ServicePort) bool {
	if len(existing) != len(required) {
		return false
	}
	for i := range required {
		existingProto := existing[i].Protocol
		if len(existingProto) == 0 {
			existingProto = corev1.ProtocolTCP
		}
		requiredProto := required[i].Protocol
		if len(requiredProto) == 0 {
			requiredProto = corev1.ProtocolTCP
		}
		if existing[i].Name != required[i].Name ||
			existingProto != requiredProto ||
			existing[i].Port != required[i].Port ||
			existing[i].TargetPort != required[i].TargetPort {
			return false
		}
	}
	return true
}
