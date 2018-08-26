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
	"k8s.io/apiserver/pkg/apis/audit"
)

// AllStages returns all possible stages
var AllStages = func() []audit.Stage {
	return []audit.Stage{
		audit.StageRequestReceived,
		audit.StageResponseStarted,
		audit.StageResponseComplete,
		audit.StagePanic,
	}
}()

// InvertOmitStages converts a blacklist of omitStages to a whitelist of stages
func InvertOmitStages(omitStages []audit.Stage) []audit.Stage {
	return SubtractStages(AllStages, omitStages)
}

// SubtractStages removes the subtractStages from the given stages
func SubtractStages(stages []audit.Stage, subtractStages []audit.Stage) []audit.Stage {
	finalStages := []audit.Stage{}
	for _, s := range stages {
		if !StageExists(subtractStages, s) {
			finalStages = append(finalStages, s)
		}
	}
	return finalStages
}

// StageExists checkes if a stage exists in an array of stages
func StageExists(stages []audit.Stage, stage audit.Stage) bool {
	for _, s := range stages {
		if stage == s {
			return true
		}
	}
	return false
}
