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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	fakeplugin "k8s.io/apiserver/plugin/pkg/audit/fake"
)

func TestEnforced(t *testing.T) {
	testCases := []struct {
		name     string
		event    *auditinternal.Event
		policy   policy.DynamicPolicy
		attribs  authorizer.Attributes
		expected []*auditinternal.Event
	}{
		{
			name: "enforce policy level",
			event: &auditinternal.Event{
				Level:          auditinternal.LevelRequestResponse,
				Stage:          auditinternal.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			policy: policy.DynamicPolicy{
				Level:  auditinternal.LevelMetadata,
				Stages: []auditinternal.Stage{auditinternal.StageResponseComplete},
			},
			attribs: authorizer.AttributesRecord{
				User: &user.DefaultInfo{
					Name: user.Anonymous,
				},
			},
			expected: []*auditinternal.Event{
				{
					Level: auditinternal.LevelMetadata,
					Stage: auditinternal.StageResponseComplete},
			},
		},
		{
			name: "enforce policy stages",
			event: &auditinternal.Event{
				Level:          auditinternal.LevelRequestResponse,
				Stage:          auditinternal.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			policy: policy.DynamicPolicy{
				Level:  auditinternal.LevelRequestResponse,
				Stages: []auditinternal.Stage{auditinternal.StageResponseStarted},
			},
			attribs: authorizer.AttributesRecord{
				User: &user.DefaultInfo{
					Name: user.Anonymous,
				},
			},
			expected: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var ev []*auditinternal.Event
			fakeBackend := fakeplugin.Backend{
				OnRequest: func(events []*auditinternal.Event) {
					ev = events
				},
			}
			b := NewBackend(&fakeBackend, &tc.policy)
			defer b.Shutdown()

			ef := &audit.EnforcedEvent{
				Event:      tc.event,
				Attributes: tc.attribs,
			}
			b.ProcessEnforcedEvents(ef)
			require.Equal(t, tc.expected, ev)
		})
	}
}
