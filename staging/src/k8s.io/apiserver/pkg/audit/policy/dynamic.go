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
	"unsafe"

	"k8s.io/api/auditregistration/v1alpha1"
	"k8s.io/apiserver/pkg/apis/audit"
)

// DynamicPolicy is an internal type that syncs with the auditregistration
// policy type
type DynamicPolicy struct {
	// The Level that all requests are recorded at.
	Level audit.Level

	// Stages is a list of stages for which events are created.
	Stages []audit.Stage
}

// ConvertRegisteredToDynamic constructs a dynamic policy api server type from a v1alpha1
func ConvertRegisteredToDynamic(p *v1alpha1.Policy) *DynamicPolicy {
	return &DynamicPolicy{
		Level:  audit.Level(p.Level),
		Stages: *(*[]audit.Stage)(unsafe.Pointer(&p.Stages)),
	}
}
