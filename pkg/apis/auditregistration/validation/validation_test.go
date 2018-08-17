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
						Level: &auditregistration.LevelRequest,
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
						Level: &auditregistration.LevelMetadata,
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
	successCases := []auditregistration.Policy{}
	successCases = append(successCases, auditregistration.Policy{}) // Empty policy
	successCases = append(successCases, auditregistration.Policy{   // Policy with omitStages and level
		Level: &auditregistration.LevelRequest,
		OmitStages: []auditregistration.Stage{
			auditregistration.Stage("RequestReceived")},
	})
	successCases = append(successCases, auditregistration.Policy{Level: &auditregistration.LevelMetadata}) // Policy with level only

	for i, policy := range successCases {
		if errs := ValidatePolicy(&policy); len(errs) != 0 {
			t.Errorf("[%d] Expected policy %#v to be valid: %v", i, policy, errs)
		}
	}

	errorCases := []auditregistration.Policy{}                                                      // Policy with missing level
	errorCases = append(errorCases, auditregistration.Policy{OmitStages: []auditregistration.Stage{ // Policy with invalid omitStages
		auditregistration.Stage("Bad")}})
	invalidLevel := auditregistration.Level("invalid")
	errorCases = append(errorCases, auditregistration.Policy{Level: &invalidLevel}) // Policy with bad level

	for i, policy := range errorCases {
		if errs := ValidatePolicy(&policy); len(errs) == 0 {
			t.Errorf("[%d] Expected policy %#v to be invalid!", i, policy)
		}
	}
}
