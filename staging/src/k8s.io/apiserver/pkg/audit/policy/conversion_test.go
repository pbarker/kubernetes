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
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
)

func TestConvertRegisteredToInternal(t *testing.T) {
	regPol := &auditregv1alpha1.Policy{
		Level: &auditregv1alpha1.LevelMetadata,
		OmitStages: []auditregv1alpha1.Stage{
			auditregv1alpha1.Stage("RequestRecieved"),
		},
	}

	expectedPolicy := &auditinternal.Policy{
		Rules: []auditinternal.PolicyRule{
			{
				Level: auditinternal.LevelMetadata,
			},
		},
		OmitStages: []auditinternal.Stage{
			auditinternal.Stage("RequestRecieved"),
		},
	}

	var p auditinternal.Policy
	err := ConvertRegisteredToInternal(regPol, &p)
	require.NoError(t, err)
	eq := reflect.DeepEqual(expectedPolicy, &p)
	require.True(t, eq)
}
