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

package policy

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/apis/audit"
)

func TestEnforcePolicy(t *testing.T) {
	testCases := []struct {
		name       string
		event      *audit.Event
		level      audit.Level
		omitStages []audit.Stage
		expected   *audit.Event
	}{
		{
			name: "enforce metadata",
			event: &audit.Event{
				Level:          audit.LevelRequestResponse,
				Stage:          audit.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			level:      audit.LevelMetadata,
			omitStages: []audit.Stage{},
			expected: &audit.Event{
				Level: audit.LevelMetadata,
				Stage: audit.StageResponseComplete,
			},
		},
		{
			name: "enforce request",
			event: &audit.Event{
				Level:          audit.LevelRequestResponse,
				Stage:          audit.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			level:      audit.LevelRequest,
			omitStages: []audit.Stage{},
			expected: &audit.Event{
				Level:         audit.LevelRequest,
				Stage:         audit.StageResponseComplete,
				RequestObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
		},
		{
			name: "enforce request response",
			event: &audit.Event{
				Level:          audit.LevelRequestResponse,
				Stage:          audit.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			level:      audit.LevelRequestResponse,
			omitStages: []audit.Stage{},
			expected: &audit.Event{
				Level:          audit.LevelRequestResponse,
				Stage:          audit.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
		},
		{
			name: "enforce omit stages",
			event: &audit.Event{
				Level:          audit.LevelRequestResponse,
				Stage:          audit.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			level:      audit.LevelRequestResponse,
			omitStages: []audit.Stage{audit.StageResponseComplete},
			expected:   nil,
		},
		{
			name: "enforce none",
			event: &audit.Event{
				Level:          audit.LevelRequestResponse,
				Stage:          audit.StageResponseComplete,
				RequestObject:  &runtime.Unknown{Raw: []byte(`test`)},
				ResponseObject: &runtime.Unknown{Raw: []byte(`test`)},
			},
			level:      audit.LevelNone,
			omitStages: []audit.Stage{},
			expected:   nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ev := EnforcePolicy(tc.event, tc.level, tc.omitStages)
			require.Equal(t, tc.expected, ev)
		})
		t.Run(fmt.Sprintf("%s_dynamic", tc.name), func(t *testing.T) {
			ev := EnforceDynamicPolicy(tc.event, tc.level, InvertOmitStages(tc.omitStages))
			require.Equal(t, tc.expected, ev)
		})
	}
}
