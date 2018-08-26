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

package enforced

import (
	"fmt"

	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
)

// PluginName is the name reported in error metrics.
const PluginName = "enforced"

// Backend filters audit events by policy trimming them as necesssary
// before forwarding them to the delegated backend
type Backend struct {
	policy          *policy.DynamicPolicy
	delegateBackend audit.Backend
}

// NewBackend returns a filtered audit backend that wraps delegate backend.
// Filtered backend automatically runs and shuts down the delegate backend.
func NewBackend(delegate audit.Backend, p *policy.DynamicPolicy) audit.EnforcedBackend {
	return &Backend{
		policy:          p,
		delegateBackend: delegate,
	}
}

// Run the delegate backend
func (b Backend) Run(stopCh <-chan struct{}) error {
	return b.delegateBackend.Run(stopCh)
}

// Shutdown the delegate backend
func (b Backend) Shutdown() {
	b.delegateBackend.Shutdown()
}

// ProcessEnforcedEvents enforces policy on the given event based on the authorizer attributes
func (b Backend) ProcessEnforcedEvents(events ...*audit.EnforcedEvent) {
	for _, event := range events {
		audit.ObservePolicyLevel(b.policy.Level)
		if b.policy.Level == auditinternal.LevelNone {
			continue
		}
		event.Event = policy.EnforceDynamicPolicy(event.Event, b.policy.Level, b.policy.Stages)
		if event.Event == nil {
			continue
		}
		b.delegateBackend.ProcessEvents(event.Event)
	}
}

// String returns a string representation of the backend
func (b Backend) String() string {
	return fmt.Sprintf("%s<%s>", PluginName, b.delegateBackend)
}
