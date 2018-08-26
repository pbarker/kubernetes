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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/apis/audit"
)

// ConvertRegisteredToInternal converts an auditregistration policy to an internal type
func ConvertRegisteredToInternal(in *v1alpha1.Policy, out *audit.Policy) error {
	out.ObjectMeta = metav1.ObjectMeta{}
	out.Rules = *(*[]audit.PolicyRule)(unsafe.Pointer(&in.Rules))
	out.OmitStages = *(*[]audit.Stage)(unsafe.Pointer(&in.OmitStages))
	return nil
}
