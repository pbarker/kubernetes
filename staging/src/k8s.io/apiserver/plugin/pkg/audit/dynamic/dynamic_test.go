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
	"testing"

	"github.com/stretchr/testify/require"

	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	webhookconfig "k8s.io/apiserver/pkg/util/webhook/config"
	bufferedplugin "k8s.io/apiserver/plugin/pkg/audit/buffered"
	fakeplugin "k8s.io/apiserver/plugin/pkg/audit/fake"
	truncateplugin "k8s.io/apiserver/plugin/pkg/audit/truncate"
	informers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDynamic(t *testing.T) {
	defaultPolicy := &auditregv1alpha1.Policy{
		Rules: []auditregv1alpha1.PolicyRule{
			{
				Level: auditregv1alpha1.LevelMetadata,
			},
		},
	}
	var internalPolicy auditinternal.Policy
	err := policy.ConvertRegisteredToInternal(defaultPolicy, &internalPolicy)
	require.NoError(t, err)
	policyChecker := policy.NewChecker(&internalPolicy)
	staticBackend := &fakeplugin.Backend{
		OnRequest: func(events []*auditinternal.Event) {},
	}
	truncateOpts := truncateplugin.NewDefaultTruncateOptions()
	bufferedOpts := bufferedplugin.NewDefaultBatchConfig()
	u := "http://localhost:4444"
	auditConfig := &auditregv1alpha1.AuditSink{
		Spec: auditregv1alpha1.AuditSinkSpec{
			Policy: defaultPolicy,
			Backend: auditregv1alpha1.Backend{
				Webhook: &auditregv1alpha1.WebhookBackend{
					ClientConfig: auditregv1alpha1.WebhookClientConfig{
						URL: &u,
					},
				},
			},
		},
	}

	testCases := []struct {
		name          string
		staticBackend audit.Backend
		auditConfig   *auditregv1alpha1.AuditSink
		expectedLen   int
	}{
		{
			name:          "should find none",
			staticBackend: nil,
			auditConfig:   nil,
			expectedLen:   0,
		},
		{
			name:          "should find static",
			staticBackend: staticBackend,
			auditConfig:   nil,
			expectedLen:   1,
		},
		{
			name:          "should find dynamic",
			staticBackend: nil,
			auditConfig:   auditConfig,
			expectedLen:   1,
		},
		{
			name:          "should find both",
			staticBackend: staticBackend,
			auditConfig:   auditConfig,
			expectedLen:   2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			stop := make(chan struct{})
			defer close(stop)

			informerFactory.Start(stop)
			informerFactory.WaitForCacheSync(stop)

			config := &Config{
				F:                       informerFactory,
				StaticPolicyChecker:     policyChecker,
				StaticBackend:           tc.staticBackend,
				TruncateConfig:          &truncateOpts,
				BufferedConfig:          &bufferedOpts,
				AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
			}
			eb, err := NewEnforcedBackend(config)
			require.NoError(t, err)

			auditInformer := informerFactory.Auditregistration().V1alpha1().AuditSinks()
			if tc.auditConfig != nil {
				auditInformer.Informer().GetIndexer().Add(tc.auditConfig)
			}
			eb.(*dynamic).updateConfiguration()
			require.Len(t, eb.(*dynamic).GetBackends(), tc.expectedLen)
		})
	}
}
