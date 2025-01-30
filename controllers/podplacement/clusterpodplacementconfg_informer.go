package podplacement

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	multiarchv1beta1 "github.com/openshift/multiarch-tuning-operator/apis/multiarch/v1beta1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	singletonSystemConfigInstance ICache
	once                          sync.Once
)

// CPPCSyncer syncs ClusterPodPlacementConfig resources using an informer.
type CPPCSyncer struct {
	mgr manager.Manager
	log logr.Logger
}

// NewCPPCSyncer creates a new CPPCSyncer.
func NewCPPCSyncer(mgr manager.Manager) *CPPCSyncer {
	return &CPPCSyncer{
		mgr: mgr,
	}
}

// Start initializes the CPPC informer and starts syncing.
func (s *CPPCSyncer) Start(ctx context.Context) error {
	s.log = log.FromContext(ctx, "handler", "CPPCSyncer")
	s.log.Info("Starting CPPC Syncer")
	mgr := s.mgr

	ic := CacheSingleton()
	// Get informer for ClusterPodPlacementConfig
	CPPCInformer, err := mgr.GetCache().GetInformerForKind(ctx, multiarchv1beta1.GroupVersion.WithKind(multiarchv1beta1.ClusterPodPlacementConfigKind))
	if err != nil {
		s.log.Error(err, "Error getting informer for ClusterPodPlacementConfig")
		return err
	}

	// Register event handlers
	_, err = CPPCInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.onAdd(ic),
		UpdateFunc: s.onUpdate(ic),
		DeleteFunc: s.onDelete(ic),
	})
	if err != nil {
		s.log.Error(err, "Error registering handler for ClusterPodPlacementConfig")
		return err
	}

	return nil
}

// onAdd handles the addition of a ClusterPodPlacementConfig.
func (s *CPPCSyncer) onAdd(ic ICache) func(obj interface{}) {
	return func(obj interface{}) {
		CPPC, ok := obj.(*multiarchv1beta1.ClusterPodPlacementConfig)
		if !ok {
			s.log.Error(errors.New("unexpected type, expected ClusterPodPlacementConfig"), "unexpected type",
				"type", fmt.Sprintf("%T", obj))
			return
		}

		err := ic.StoreClusterPodPlacementConfig(CPPC)
		if err != nil {
			s.log.Error(err, "Error updating ClusterPodPlacementConfig",
				"CPPC name", CPPC.Name)
		} else {
			s.log.Info("Added ClusterPodPlacementConfig", "CPPC name", CPPC.Name, "namespace", CPPC.Namespace)
		}
	}
}

// onDelete handles the deletion of a ClusterPodPlacementConfig.
func (s *CPPCSyncer) onDelete(ic ICache) func(obj interface{}) {
	return func(obj interface{}) {

		CPPC, ok := obj.(*multiarchv1beta1.ClusterPodPlacementConfig)
		if !ok {
			s.log.Error(errors.New("unexpected type, expected ClusterPodPlacementConfig"), "unexpected type",
				"type", fmt.Sprintf("%T", obj))
			return
		}

		err := ic.DeleteClusterPodPlacementConfig()
		if err != nil {
			s.log.Error(err, "Error deleting ClusterPodPlacementConfig",
				"name", CPPC.Name)
		} else {
			s.log.Info("Deleted ClusterPodPlacementConfig", "name", CPPC.Name, "namespace", CPPC.Namespace)
		}
	}
}

// onUpdate handles updates to a ClusterPodPlacementConfig.
func (s *CPPCSyncer) onUpdate(ic ICache) func(oldObj, newObj interface{}) {
	return func(oldobj, newobj interface{}) {
		oldConfig, ok := oldobj.(*multiarchv1beta1.ClusterPodPlacementConfig)

		if !ok {
			s.log.Error(errors.New("unexpected type, expected ClusterPodPlacementConfig"), "unexpected type",
				"type", fmt.Sprintf("%T", oldobj))
			return
		}

		newConfig, ok := newobj.(*multiarchv1beta1.ClusterPodPlacementConfig)
		if !ok {
			s.log.Error(errors.New("unexpected type, expected ClusterPodPlacementConfig"), "unexpected type",
				"type", fmt.Sprintf("%T", newobj))
			return
		}

		if oldConfig.ResourceVersion == newConfig.ResourceVersion {
			return
		}
		s.onAdd(ic)(newobj)
	}
}

type ICache interface {
	// StoreClusterPodPlacementConfig stores the clusterpodplacementconfig and webhook in a struct
	StoreClusterPodPlacementConfig(config *multiarchv1beta1.ClusterPodPlacementConfig) error

	DeleteClusterPodPlacementConfig() error

	GetClusterPodPlacementConfig() *multiarchv1beta1.ClusterPodPlacementConfig
}

type clusterPodPlacementConfig struct {
	config *multiarchv1beta1.ClusterPodPlacementConfig
	mu     sync.Mutex // Mutex for `config`
}

func CacheSingleton() ICache {
	once.Do(func() {
		singletonSystemConfigInstance = newCache()
	})
	return singletonSystemConfigInstance
}

func newCache() ICache {
	c := &clusterPodPlacementConfig{
		config: nil,
	}
	return c
}

func (c *clusterPodPlacementConfig) StoreClusterPodPlacementConfig(config *multiarchv1beta1.ClusterPodPlacementConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = config.DeepCopy()
	return nil
}

func (c *clusterPodPlacementConfig) DeleteClusterPodPlacementConfig() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = nil
	return nil
}

func (c *clusterPodPlacementConfig) GetClusterPodPlacementConfig() *multiarchv1beta1.ClusterPodPlacementConfig {
	return c.config
}
