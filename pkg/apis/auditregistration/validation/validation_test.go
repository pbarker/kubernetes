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

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/apis/auditregistration"
)

func TestValidateAuditSink(t *testing.T) {
	testURL := "localhost"
	testCases := []struct {
		name   string
		conf   auditregistration.AuditSink
		numErr int
	}{
		{
			name: "should pass full config",
			conf: auditregistration.AuditSink{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myconf",
				},
				Spec: auditregistration.AuditSinkSpec{
					Policy: &auditregistration.Policy{
						Rules: []auditregistration.PolicyRule{
							{
								Level: auditregistration.LevelRequest,
							},
						},
						OmitStages: []auditregistration.Stage{
							auditregistration.StageRequestReceived,
						},
					},
					Backend: auditregistration.Backend{
						Webhook: &auditregistration.WebhookBackend{
							ClientConfig: auditregistration.WebhookClientConfig{
								URL: &testURL,
							},
						},
					},
				},
			},
			numErr: 0,
		},
		{
			name: "should pass no policy",
			conf: auditregistration.AuditSink{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myconf",
				},
				Spec: auditregistration.AuditSinkSpec{
					Backend: auditregistration.Backend{
						Webhook: &auditregistration.WebhookBackend{
							ClientConfig: auditregistration.WebhookClientConfig{
								URL: &testURL,
							},
						},
					},
				},
			},
			numErr: 0,
		},
		{
			name: "should fail no webhook",
			conf: auditregistration.AuditSink{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myconf",
				},
				Spec: auditregistration.AuditSinkSpec{
					Policy: &auditregistration.Policy{
						Rules: []auditregistration.PolicyRule{
							{
								Level: auditregistration.LevelRequest,
							},
						},
						OmitStages: []auditregistration.Stage{
							auditregistration.StageRequestReceived,
						},
					},
					Backend: auditregistration.Backend{},
				},
			},
			numErr: 1,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			errs := ValidateAuditSink(&test.conf)
			require.Len(t, errs, test.numErr)
		})
	}
}

func TestValidatePolicy(t *testing.T) {
	validRules := []auditregistration.PolicyRule{
		{ // Defaulting rule
			Level: auditregistration.LevelMetadata,
		}, { // Matching non-humans
			Level:      auditregistration.LevelNone,
			UserGroups: []string{"system:serviceaccounts", "system:nodes"},
		}, { // Specific request
			Level:      auditregistration.LevelRequestResponse,
			Verbs:      []string{"get"},
			Resources:  []auditregistration.GroupResources{{Group: "rbac.authorization.k8s.io", Resources: []string{"roles", "rolebindings"}}},
			Namespaces: []string{"kube-system"},
		}, { // Some non-resource URLs
			Level:      auditregistration.LevelMetadata,
			UserGroups: []string{"developers"},
			NonResourceURLs: []string{
				"/logs*",
				"/healthz*",
				"/metrics",
				"*",
			},
		}, { // Omit RequestReceived stage
			Level: auditregistration.LevelMetadata,
			OmitStages: []auditregistration.Stage{
				auditregistration.Stage("RequestReceived"),
			},
		},
	}
	successCases := []auditregistration.Policy{}
	for _, rule := range validRules {
		successCases = append(successCases, auditregistration.Policy{Rules: []auditregistration.PolicyRule{rule}})
	}
	successCases = append(successCases, auditregistration.Policy{})                                     // Empty policy is valid.
	successCases = append(successCases, auditregistration.Policy{OmitStages: []auditregistration.Stage{ // Policy with omitStages
		auditregistration.Stage("RequestReceived")}})
	successCases = append(successCases, auditregistration.Policy{Rules: validRules}) // Multiple rules.

	for i, policy := range successCases {
		if errs := ValidatePolicy(&policy); len(errs) != 0 {
			t.Errorf("[%d] Expected policy %#v to be valid: %v", i, policy, errs)
		}
	}

	invalidRules := []auditregistration.PolicyRule{
		{}, // Empty rule (missing Level)
		{ // Missing level
			Verbs:      []string{"get"},
			Resources:  []auditregistration.GroupResources{{Resources: []string{"secrets"}}},
			Namespaces: []string{"kube-system"},
		}, { // Invalid Level
			Level: "FooBar",
		}, { // NonResourceURLs + Namespaces
			Level:           auditregistration.LevelMetadata,
			Namespaces:      []string{"default"},
			NonResourceURLs: []string{"/logs*"},
		}, { // NonResourceURLs + ResourceKinds
			Level:           auditregistration.LevelMetadata,
			Resources:       []auditregistration.GroupResources{{Resources: []string{"secrets"}}},
			NonResourceURLs: []string{"/logs*"},
		}, { // invalid group name
			Level:     auditregistration.LevelMetadata,
			Resources: []auditregistration.GroupResources{{Group: "rbac.authorization.k8s.io/v1beta1", Resources: []string{"roles"}}},
		}, { // invalid non-resource URLs
			Level: auditregistration.LevelMetadata,
			NonResourceURLs: []string{
				"logs",
				"/healthz*",
			},
		}, { // empty non-resource URLs
			Level: auditregistration.LevelMetadata,
			NonResourceURLs: []string{
				"",
				"/healthz*",
			},
		}, { // invalid non-resource URLs with multi "*"
			Level: auditregistration.LevelMetadata,
			NonResourceURLs: []string{
				"/logs/*/*",
				"/metrics",
			},
		}, { // invalid non-resrouce URLs with "*" not in the end
			Level: auditregistration.LevelMetadata,
			NonResourceURLs: []string{
				"/logs/*.log",
				"/metrics",
			},
		},
		{ // ResourceNames without Resources
			Level:      auditregistration.LevelMetadata,
			Verbs:      []string{"get"},
			Resources:  []auditregistration.GroupResources{{ResourceNames: []string{"leader"}}},
			Namespaces: []string{"kube-system"},
		},
		{ // invalid omitStages in rule
			Level: auditregistration.LevelMetadata,
			OmitStages: []auditregistration.Stage{
				auditregistration.Stage("foo"),
			},
		},
	}
	errorCases := []auditregistration.Policy{}
	for _, rule := range invalidRules {
		errorCases = append(errorCases, auditregistration.Policy{Rules: []auditregistration.PolicyRule{rule}})
	}

	// Multiple rules.
	errorCases = append(errorCases, auditregistration.Policy{Rules: append(validRules, auditregistration.PolicyRule{})})

	// invalid omitStages in policy
	policy := auditregistration.Policy{OmitStages: []auditregistration.Stage{
		auditregistration.Stage("foo"),
	},
		Rules: []auditregistration.PolicyRule{
			{
				Level: auditregistration.LevelMetadata,
			},
		},
	}
	errorCases = append(errorCases, policy)

	for i, policy := range errorCases {
		if errs := ValidatePolicy(&policy); len(errs) == 0 {
			t.Errorf("[%d] Expected policy %#v to be invalid!", i, policy)
		}
	}
}
