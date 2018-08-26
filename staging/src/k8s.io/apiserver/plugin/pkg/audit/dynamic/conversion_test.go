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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	auditv1alpha1 "k8s.io/apiserver/pkg/apis/audit/v1alpha1"
	webhookconfig "k8s.io/apiserver/pkg/util/webhook/config"
	bufferedplugin "k8s.io/apiserver/plugin/pkg/audit/buffered"
	fakeplugin "k8s.io/apiserver/plugin/pkg/audit/fake"
	truncateplugin "k8s.io/apiserver/plugin/pkg/audit/truncate"
	informers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestToBackend(t *testing.T) {
	client := fake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	stop := make(chan struct{})
	defer close(stop)

	informerFactory.Start(stop)
	informerFactory.WaitForCacheSync(stop)
	defaultPolicy := &auditregv1alpha1.Policy{
		Level: auditregv1alpha1.LevelMetadata,
	}
	staticBackend := &fakeplugin.Backend{
		OnRequest: func(events []*auditinternal.Event) {},
	}
	require.NoError(t, err)
	truncateOpts := truncateplugin.NewDefaultTruncateOptions()
	bufferedOpts := bufferedplugin.NewDefaultBatchConfig()
	u := "http://localhost:4444"
	testCases := []struct {
		name          string
		auditConfig   *auditregv1alpha1.AuditSink
		dynamicConfig *Config
		shouldErr     bool
	}{
		{
			name: "should pass",
			auditConfig: &auditregv1alpha1.AuditSink{
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
			},
			dynamicConfig: &Config{
				F:                       informerFactory,
				StaticPolicyChecker:     policyChecker,
				StaticBackend:           staticBackend,
				TruncateConfig:          &truncateOpts,
				BufferedConfig:          &bufferedOpts,
				AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
			},
			shouldErr: false,
		},
		{
			name: "should pass no trucate",
			auditConfig: &auditregv1alpha1.AuditSink{
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
			},
			dynamicConfig: &Config{
				F:                       informerFactory,
				StaticPolicyChecker:     policyChecker,
				StaticBackend:           staticBackend,
				TruncateConfig:          nil,
				BufferedConfig:          &bufferedOpts,
				AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
			},
			shouldErr: false,
		},
		{
			name: "should pass no buffered",
			auditConfig: &auditregv1alpha1.AuditSink{
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
			},
			dynamicConfig: &Config{
				F:                       informerFactory,
				StaticPolicyChecker:     policyChecker,
				StaticBackend:           staticBackend,
				TruncateConfig:          &truncateOpts,
				BufferedConfig:          nil,
				AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
			},
			shouldErr: false,
		},
		{
			name: "should pass missing static policy",
			auditConfig: &auditregv1alpha1.AuditSink{
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
			},
			dynamicConfig: &Config{
				F:                       informerFactory,
				StaticBackend:           staticBackend,
				TruncateConfig:          &truncateOpts,
				BufferedConfig:          &bufferedOpts,
				AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
			},
			shouldErr: false,
		},
		{
			name: "should fail no policies",
			auditConfig: &auditregv1alpha1.AuditSink{
				Spec: auditregv1alpha1.AuditSinkSpec{
					Policy: nil,
					Backend: auditregv1alpha1.Backend{
						Webhook: &auditregv1alpha1.WebhookBackend{
							ClientConfig: auditregv1alpha1.WebhookClientConfig{
								URL: &u,
							},
						},
					},
				},
			},
			dynamicConfig: &Config{
				F:                       informerFactory,
				StaticBackend:           staticBackend,
				TruncateConfig:          &truncateOpts,
				BufferedConfig:          &bufferedOpts,
				AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
			},
			shouldErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eb, err := NewEnforcedBackend(tc.dynamicConfig)
			require.NoError(t, err)
			c := converter{eb.(*dynamic), tc.auditConfig}
			ef, err := c.ToBackend()
			if tc.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, ef)
			}

		})
	}
}

func TestBuildWebhookBackend(t *testing.T) {
	defaultPolicy := &auditregv1alpha1.Policy{
		Rules: []auditregv1alpha1.PolicyRule{
			{
				Level: auditregv1alpha1.LevelMetadata,
			},
		},
	}

	auditScheme := runtime.NewScheme()
	err := auditv1alpha1.AddToScheme(auditScheme)
	require.NoError(t, err)
	negotiatedSerializer := serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{
		Serializer: serializer.NewCodecFactory(auditScheme).LegacyCodec(auditv1alpha1.SchemeGroupVersion),
	})

	u := "http://localhost:4444"
	testCases := []struct {
		name        string
		auditConfig *auditregv1alpha1.AuditSink
		shouldErr   bool
	}{
		{
			name: "should build",
			auditConfig: &auditregv1alpha1.AuditSink{
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
			},
			shouldErr: false,
		},
		{
			name: "should fail missing url",
			auditConfig: &auditregv1alpha1.AuditSink{
				Spec: auditregv1alpha1.AuditSinkSpec{
					Policy: defaultPolicy,
					Backend: auditregv1alpha1.Backend{
						Webhook: &auditregv1alpha1.WebhookBackend{
							ClientConfig: auditregv1alpha1.WebhookClientConfig{},
						},
					},
				},
			},
			shouldErr: true,
		},
		{
			name: "should fail missing webhook",
			auditConfig: &auditregv1alpha1.AuditSink{
				Spec: auditregv1alpha1.AuditSinkSpec{
					Policy:  defaultPolicy,
					Backend: auditregv1alpha1.Backend{},
				},
			},
			shouldErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := &dynamic{
				config: &Config{
					AuthInfoResolverWrapper: webhookconfig.DefaultDynamicResolveWrapper,
				},
				negotiatedSerializer: negotiatedSerializer,
			}
			c := &converter{d, tc.auditConfig}
			ab, err := c.buildWebhookBackend()
			if tc.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, ab)
			}
		})
	}
}
