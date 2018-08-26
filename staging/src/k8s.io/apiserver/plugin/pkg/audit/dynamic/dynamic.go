/*
Copyright 2018 The Kubernetes Authors.

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

package dynamic

import (
	"sync"

	"github.com/golang/glog"
	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	webhook "k8s.io/apiserver/pkg/util/webhook"
	bufferedplugin "k8s.io/apiserver/plugin/pkg/audit/buffered"
	enforcedplugin "k8s.io/apiserver/plugin/pkg/audit/enforced"
	truncateplugin "k8s.io/apiserver/plugin/pkg/audit/truncate"
	informers "k8s.io/client-go/informers"
	auditlisters "k8s.io/client-go/listers/auditregistration/v1alpha1"
	"k8s.io/client-go/tools/cache"
)

const (
	// PluginName is the name reported in error metrics.
	PluginName = "dynamic"

	staticUID = types.UID("static")
)

// Config holds the configuration for the dynamic backend
type Config struct {
	// F Informer factory for the audit configrations
	F informers.SharedInformerFactory
	// StaticPolicyChecker is the runtime configured policy if exists
	StaticPolicyChecker policy.Checker
	// StaticBackend is the runtime configured backend if exists
	StaticBackend audit.Backend
	// TruncateConfig is the runtime truncate configuration
	TruncateConfig *truncateplugin.Config
	// BufferedConfig is the runtime buffered configuration
	BufferedConfig *bufferedplugin.BatchConfig
	// AuthInfoReolverWrapper provides the webhook authentication for in-cluster endpoints
	AuthInfoResolverWrapper webhook.AuthenticationInfoResolverWrapper
}

// Backend represents a dynamic backend
type Backend struct {
	configuration   *auditregv1alpha1.AuditSink
	enforcedBackend audit.EnforcedBackend
	stopChan        chan struct{}
}

// NewEnforcedBackend returns an enforced backend that dynamically updates its configuration based on a
// shared informer. It takes the static policy and static backends configured by the runtime flags
// as arguments and runs them with policy enforcment.
func NewEnforcedBackend(c *Config) (audit.EnforcedBackend, error) {
	informer := c.F.Auditregistration().V1alpha1().AuditSinks()

	if c.BufferedConfig == nil {
		defaultConfig := bufferedplugin.NewDefaultBatchConfig()
		c.BufferedConfig = &defaultConfig
	}

	backends := make(map[types.UID]*Backend)
	var staticBackend *Backend
	if c.StaticBackend != nil {
		enforced := enforcedplugin.NewBackend(c.StaticBackend, c.StaticPolicyChecker)
		staticBackend = &Backend{nil, enforced, make(chan struct{})}
		backends[staticUID] = staticBackend
	}

	cm, err := webhook.NewClientManager(auditregv1alpha1.SchemeGroupVersion, auditregv1alpha1.AddToScheme)
	if err != nil {
		return nil, err
	}
	cm.SetAuthenticationInfoResolverWrapper(c.AuthInfoResolverWrapper)

	auditScheme := runtime.NewScheme()
	if err := auditinternal.AddToScheme(auditScheme); err != nil {
		return nil, err
	}
	negotiatedSerializer := serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{
		Serializer: serializer.NewCodecFactory(auditScheme).LegacyCodec(auditinternal.SchemeGroupVersion),
	})

	manager := &dynamic{
		config:               c,
		staticBackend:        staticBackend,
		backends:             syncedBackends{idMap: backends},
		updater:              updater{},
		lister:               informer.Lister(),
		hasSynced:            informer.Informer().HasSynced,
		negotiatedSerializer: negotiatedSerializer,
		webhookClientManager: cm,
	}

	// On any change, rebuild the config
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { manager.updateConfiguration() },
		UpdateFunc: func(_, _ interface{}) { manager.updateConfiguration() },
		DeleteFunc: func(_ interface{}) { manager.updateConfiguration() },
	})

	return manager, nil
}

type dynamic struct {
	config               *Config
	staticBackend        *Backend
	backends             syncedBackends
	updater              updater
	lister               auditlisters.AuditSinkLister
	hasSynced            func() bool
	negotiatedSerializer runtime.NegotiatedSerializer
	webhookClientManager webhook.ClientManager
}

type syncedBackends struct {
	idMap map[types.UID]*Backend
	sync.RWMutex
}

// updater forces configuration updates to run serially. This is a separate lock
// so that events are not blocked while the backends build
type updater struct{ sync.Mutex }

// ProcessEnforcedEvents proccesses the given events per current backend map
func (d *dynamic) ProcessEnforcedEvents(events ...*audit.EnforcedEvent) {
	for _, b := range d.GetBackends() {
		b.enforcedBackend.ProcessEnforcedEvents(events...)
	}
}

// Run the dynamic backends
func (d *dynamic) Run(stopCh <-chan struct{}) error {
	// Shutdown channels are handled individually with the dynamic
	// backend, message must be propagated out
	go func() {
		for {
			select {
			case <-stopCh:
				d.Shutdown()
				return
			default:
			}
		}
	}()
	var funcs []func() error
	for _, b := range d.GetBackends() {
		funcs = append(funcs, func() error {
			return b.enforcedBackend.Run(b.stopChan)
		})
	}
	return errors.AggregateGoroutines(funcs...)
}

// Shutdown closes all backends stop channels and then runs their
// subsequent shutdown methods
func (d *dynamic) Shutdown() {
	for _, b := range d.GetBackends() {
		close(b.stopChan)
		b.enforcedBackend.Shutdown()
	}
}

// HasSynced tells whether the informer has synced
func (d *dynamic) HasSynced() bool {
	return d.hasSynced()
}

// GetBackends retrieves current backends in a safe manner
func (d *dynamic) GetBackends() map[types.UID]*Backend {
	d.backends.RLock()
	defer d.backends.RUnlock()
	return d.backends.idMap
}

// SetBackends sets the current backends in a safe manner
func (d *dynamic) SetBackends(backends map[types.UID]*Backend) {
	d.backends.Lock()
	defer d.backends.Unlock()
	d.backends.idMap = backends
	return
}

// updateConfigurations is called from the shared informer and will compare the
// current configurations found to the existing ones held. It will create/start
// any new or updated backends, and remove/stop any deleted ones.
func (d *dynamic) updateConfiguration() {
	// lock to force serial execution
	d.updater.Lock()
	defer d.updater.Unlock()

	configurations, err := d.lister.List(labels.Everything())
	if err != nil {
		audit.HandlePluginError(PluginName, err)
		return
	}

	// copy the current state
	currentBackends := make(map[types.UID]*Backend)
	for u, c := range d.GetBackends() {
		currentBackends[u] = c
	}

	newBackends := map[types.UID]*Backend{}
	for _, configuration := range configurations {
		// check if configuration currently exists
		if current, ok := currentBackends[configuration.ObjectMeta.UID]; ok {
			// check if object has changed
			if current.configuration.ObjectMeta.ResourceVersion != configuration.ObjectMeta.ResourceVersion {
				// update
				backend, err := d.createBackend(configuration)
				if err != nil {
					// should we revert to the old backend here?
					audit.HandlePluginError(PluginName, err)
					continue
				}
				newBackends[configuration.ObjectMeta.UID] = backend
				continue
			}
			// keep current
			newBackends[configuration.ObjectMeta.UID] = current
			delete(currentBackends, configuration.ObjectMeta.UID)
			continue
		}
		// create new
		backend, err := d.createBackend(configuration)
		if err != nil {
			audit.HandlePluginError(PluginName, err)
			continue
		}
		newBackends[configuration.ObjectMeta.UID] = backend
	}
	names := []string{}
	for _, backends := range newBackends {
		names = append(names, backends.configuration.Name)
	}
	if d.staticBackend != nil {
		newBackends[staticUID] = d.staticBackend
		delete(currentBackends, staticUID)
	}

	d.SetBackends(newBackends)
	glog.Infof("updated dynamic audit backends: %+v", names)

	// shutdown removed backends
	for _, backend := range currentBackends {
		close(backend.stopChan)
		backend.enforcedBackend.Shutdown()
	}
	return
}

// createBackend will convert an audit sink configuration to a Backend and run the backend
func (d *dynamic) createBackend(configuration *auditregv1alpha1.AuditSink) (*Backend, error) {
	conv := converter{d, configuration}
	backend, err := conv.ToBackend()
	if err != nil {
		return nil, err
	}
	err = backend.enforcedBackend.Run(backend.stopChan)
	if err != nil {
		return nil, err
	}
	return backend, nil
}
