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
	"fmt"
	"time"

	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	"k8s.io/apiserver/pkg/util/webhook/config"
	bufferedplugin "k8s.io/apiserver/plugin/pkg/audit/buffered"
	enforcedplugin "k8s.io/apiserver/plugin/pkg/audit/enforced"
	truncateplugin "k8s.io/apiserver/plugin/pkg/audit/truncate"
	webhookplugin "k8s.io/apiserver/plugin/pkg/audit/webhook"
)

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
	if c.ac.Spec.Backend.Webhook == nil {
		return nil, fmt.Errorf("skipping configuration, no webhook supplied")
	}
	cm, err := config.NewClientManager(c.negotiatedSerializer)
	if err != nil {
		return nil, fmt.Errorf("error updating configuration: %v", err)
	}
	authResolver := c.config.AuthInfoResolverWrapper(config.NewDynamicAuthenticationInfoResolver())
	cm.SetAuthenticationInfoResolver(authResolver)
	cm.SetServiceResolver(config.NewDefaultServiceResolver())
	wh := config.ConvertAuditConfigToGeneric(&c.ac.Spec.Backend.Webhook.ClientConfig)
	cli, err := cm.HookClient(wh)
	if err != nil {
		return nil, fmt.Errorf("error updating configuration: %v", err)
	}
	backoffDuration := webhookplugin.DefaultInitialBackoff
	if c.ac.Spec.Backend.Webhook.InitialBackoff != nil {
		backoffDuration = time.Second * time.Duration(*c.ac.Spec.Backend.Webhook.InitialBackoff)
	}

	backend := webhookplugin.NewDynamicBackend(cli, backoffDuration)
	return backend, nil
}

func (c *converter) applyBufferedOpts(delegate audit.Backend) audit.Backend {
	bc := c.config.BufferedConfig
	tc := c.ac.Spec.Backend.Webhook.ThrottleConfig
	if tc != nil {
		bc.ThrottleEnable = true
		if tc.ThrottleBurst != nil {
			bc.ThrottleBurst = int(*tc.ThrottleBurst)
		}
		if tc.ThrottleQPS != nil {
			bc.ThrottleQPS = float32(*tc.ThrottleQPS)
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
	// if policy is nil apply the static policy if exists
	var policyChecker policy.Checker
	if c.ac.Spec.Policy == nil {
		if c.config.StaticPolicyChecker == nil {
			return nil, fmt.Errorf("error updating configuration: no policy provided")
		}
		policyChecker = c.config.StaticPolicyChecker
	} else {
		var pol auditinternal.Policy
		err := policy.ConvertRegisteredToInternal(c.ac.Spec.Policy, &pol)
		if err != nil {
			return nil, err
		}
		policyChecker = policy.NewChecker(&pol)
	}
	eb := enforcedplugin.NewBackend(delegate, policyChecker)
	return eb, nil
}
