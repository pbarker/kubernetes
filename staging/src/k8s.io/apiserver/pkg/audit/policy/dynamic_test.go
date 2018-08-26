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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/api/auditregistration/v1alpha1"
	"k8s.io/apiserver/pkg/apis/audit"
)

func TestConvertRegisteredToDynamic(t *testing.T) {
	for _, test := range []struct {
		desc       string
		registered *v1alpha1.Policy
		dynamic    *DynamicPolicy
	}{
		{
			desc: "should convert full",
			registered: &v1alpha1.Policy{
				Level: v1alpha1.LevelMetadata,
				Stages: []v1alpha1.Stage{
					v1alpha1.StageResponseComplete,
				},
			},
			dynamic: &DynamicPolicy{
				Level: audit.LevelMetadata,
				Stages: []audit.Stage{
					audit.StageResponseComplete,
				},
			},
		},
		{
			desc: "should convert missing level",
			registered: &v1alpha1.Policy{
				Stages: []v1alpha1.Stage{
					v1alpha1.StageResponseComplete,
				},
			},
			dynamic: &DynamicPolicy{
				Stages: []audit.Stage{
					audit.StageResponseComplete,
				},
			},
		},
		{
			desc: "should convert missing stages",
			registered: &v1alpha1.Policy{
				Level: v1alpha1.LevelMetadata,
			},
			dynamic: &DynamicPolicy{
				Level: audit.LevelMetadata,
			},
		},
		{
			desc:       "should convert empty",
			registered: &v1alpha1.Policy{},
			dynamic:    &DynamicPolicy{},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			d := ConvertRegisteredToDynamic(test.registered)
			require.Equal(t, d, test.dynamic)
		})
	}
}
