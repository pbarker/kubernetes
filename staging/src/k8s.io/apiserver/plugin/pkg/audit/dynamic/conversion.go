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
	"time"

	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	auditutil "k8s.io/apiserver/pkg/audit/util"
	"k8s.io/apiserver/pkg/util/webhook"
	bufferedplugin "k8s.io/apiserver/plugin/pkg/audit/buffered"
	enforcedplugin "k8s.io/apiserver/plugin/pkg/audit/enforced"
	truncateplugin "k8s.io/apiserver/plugin/pkg/audit/truncate"
	webhookplugin "k8s.io/apiserver/plugin/pkg/audit/webhook"
)

const retryBackoff = 500 * time.Millisecond

// converter converts the AuditSink object into backends
type converter struct {
	*dynamic
	ac *auditregv1alpha1.AuditSink
}

// ToBackend creates a Backend from the AuditSink object
func (c *converter) ToBackend() (*Backend, error) {
	backend, err := c.buildWebhookBackend()
	if err != nil {
		return nil, err
	}
	backend = c.applyBufferedOpts(backend)
	backend = c.applyTrucateOpts(backend)
	eb, err := c.applyEnforcedOpts(backend)
	if err != nil {
		return nil, err
	}
	ch := make(chan struct{})
	return &Backend{
		configuration:   c.ac,
		enforcedBackend: eb,
		stopChan:        ch,
	}, nil
}

func (c *converter) buildWebhookBackend() (audit.Backend, error) {
	hookClient := auditutil.HookClientConfigForSink(c.ac)
	cli, err := c.webhookClientManager.HookClient(hookClient)
	if err != nil {
		return nil, &webhook.ErrCallingWebhook{WebhookName: c.ac.Name, Reason: err}
	}

	backend := webhookplugin.NewDynamicBackend(cli, retryBackoff)
	return backend, nil
}

func (c *converter) applyBufferedOpts(delegate audit.Backend) audit.Backend {
	bc := c.config.BufferedConfig
	tc := c.ac.Spec.Webhook.Throttle
	if tc != nil {
		bc.ThrottleEnable = true
		if tc.Burst != nil {
			bc.ThrottleBurst = int(*tc.Burst)
		}
		if tc.QPS != nil {
			bc.ThrottleQPS = float32(*tc.QPS)
		}
	} else {
		bc.ThrottleEnable = false
	}
	return bufferedplugin.NewBackend(delegate, *bc)
}

func (c *converter) applyTrucateOpts(delegate audit.Backend) audit.Backend {
	if c.config.TruncateConfig != nil {
		return truncateplugin.NewBackend(delegate, *c.config.TruncateConfig, auditinternal.SchemeGroupVersion)
	}
	return delegate
}

func (c *converter) applyEnforcedOpts(delegate audit.Backend) (audit.EnforcedBackend, error) {
	pol := policy.ConvertRegisteredToDynamic(&c.ac.Spec.Policy)
	eb := enforcedplugin.NewBackend(delegate, pol)
	return eb, nil
}
