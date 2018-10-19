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
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/apis/audit"
	auditfuzz "k8s.io/apiserver/pkg/apis/audit/fuzzer"
)

func TestEnforcePolicy(t *testing.T) {
	levels := AllLevels()
	scheme := runtime.NewScheme()
	audit.SchemeBuilder.AddToScheme(scheme)
	codecs := runtimeserializer.NewCodecFactory(scheme)
	rs := rand.NewSource(time.Now().UnixNano())
	objectFuzzer := fuzzer.FuzzerFor(auditfuzz.Funcs, rs, codecs)

	for _, tc := range []struct {
		name       string
		level      audit.Level
		omitStages []audit.Stage
	}{
		{
			name:  "level metadata",
			level: audit.LevelMetadata,
		},
		{
			name:  "level request",
			level: audit.LevelRequest,
		},
		{
			name:  "level requestresponse",
			level: audit.LevelRequestResponse,
		},
		{
			name:  "level none",
			level: audit.LevelNone,
		},
		{
			name:  "level unknown",
			level: audit.Level("unknown"),
		},
		{
			name:       "stage valid",
			level:      audit.LevelRequest,
			omitStages: []audit.Stage{audit.StageRequestReceived},
		},
		{
			name:       "stage unknown",
			level:      audit.LevelRequest,
			omitStages: []audit.Stage{"unknown"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := make([]audit.Event, 20)
			omitSet := sets.NewString(ConvertStagesToStrings(tc.omitStages)...)
			for i := range events {
				e := &events[i]
				objectFuzzer.Fuzz(e)
				ev, err := EnforcePolicy(e, tc.level, tc.omitStages)
				if omitSet.Has(string(e.Stage)) {
					require.Nil(t, err)
					require.Nil(t, ev)
				}
				if !levels.Has(string(tc.level)) {
					require.NotNil(t, err)
					require.Nil(t, ev)
					return
				}
				require.Nil(t, err)
				if tc.level == audit.LevelNone {
					require.Nil(t, ev)
					return
				}
				switch ev.Level {
				case audit.LevelMetadata:
					require.Nil(t, ev.ResponseObject)
					require.Nil(t, ev.RequestObject)
				case audit.LevelRequest:
					require.Nil(t, ev.ResponseObject)
				}
				require.Equal(t, ev.Level, tc.level)
			}
		})
	}

}
