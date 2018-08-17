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
	"strings"

	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/auditregistration"
)

// ValidateAuditSink validates the AuditSinks
func ValidateAuditSink(as *auditregistration.AuditSink) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&as.ObjectMeta, false, genericvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateAuditSinkSpec(as.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateAuditSinkSpec validates the sink spec for audit
func ValidateAuditSinkSpec(s auditregistration.AuditSinkSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if s.Policy != nil {
		allErrs = append(allErrs, ValidatePolicy(s.Policy)...)
	}
	if s.Backend.Webhook == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, s.Backend.Webhook, "must provide webhook backend"))
		return allErrs
	}
	allErrs = append(allErrs, ValidateWebhookBackend(s.Backend.Webhook, field.NewPath("webhook"))...)
	return allErrs
}

// ValidateWebhookBackend validates the webhook backend
func ValidateWebhookBackend(b *auditregistration.WebhookBackend, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if b.ThrottleConfig != nil {
		allErrs = append(allErrs, ValidateWebhookThrottleConfig(b.ThrottleConfig, fldPath.Child("throttleConfig"))...)
	}
	allErrs = append(allErrs, ValidateWebhookClientConfig(&b.ClientConfig, fldPath.Child("clientConfig"))...)
	return allErrs
}

// ValidateWebhookThrottleConfig validates the throttle config
func ValidateWebhookThrottleConfig(c *auditregistration.WebhookThrottleConfig, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if c.ThrottleQPS != nil && *c.ThrottleQPS <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("throttleQPS"), c.ThrottleQPS, "throttleQPS must be a positive number"))
	}
	if c.ThrottleBurst != nil && *c.ThrottleBurst <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("throttleBurst"), c.ThrottleBurst, "throttleBurst must be a positive number"))
	}
	return allErrs
}

// ValidateWebhookClientConfig validates the WebhookClientConfig
func ValidateWebhookClientConfig(c *auditregistration.WebhookClientConfig, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if c.URL == nil && c.Service == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, c, "either url or service definition must be set for webhook"))
	}
	if c.URL != nil && c.Service != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, c, "both url and service definition should not be set, choose one"))
	}
	return allErrs
}

// ValidatePolicy validates the audit policy
func ValidatePolicy(policy *auditregistration.Policy) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateOmitStages(policy.OmitStages, field.NewPath("omitStages"))...)
	if policy.Level != nil {
		allErrs = append(allErrs, validateLevel(*policy.Level, field.NewPath("level"))...)
	}
	return allErrs
}

var validLevels = []string{
	string(auditregistration.LevelNone),
	string(auditregistration.LevelMetadata),
	string(auditregistration.LevelRequest),
	string(auditregistration.LevelRequestResponse),
}

var validOmitStages = []string{
	string(auditregistration.StageRequestReceived),
	string(auditregistration.StageResponseStarted),
	string(auditregistration.StageResponseComplete),
	string(auditregistration.StagePanic),
}

func validateLevel(level auditregistration.Level, fldPath *field.Path) field.ErrorList {
	switch level {
	case auditregistration.LevelNone, auditregistration.LevelMetadata, auditregistration.LevelRequest, auditregistration.LevelRequestResponse:
		return nil
	case "":
		return field.ErrorList{field.Required(fldPath, "")}
	default:
		return field.ErrorList{field.NotSupported(fldPath, level, validLevels)}
	}
}

func validateOmitStages(omitStages []auditregistration.Stage, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for i, stage := range omitStages {
		valid := false
		for _, validOmitStage := range validOmitStages {
			if string(stage) == validOmitStage {
				valid = true
				break
			}
		}
		if !valid {
			allErrs = append(allErrs, field.Invalid(fldPath.Index(i), string(stage), "allowed stages are "+strings.Join(validOmitStages, ",")))
		}
	}
	return allErrs
}

// ValidateAuditSinkUpdate validates an update to the object
func ValidateAuditSinkUpdate(newC, oldC *auditregistration.AuditSink) field.ErrorList {
	return ValidateAuditSink(newC)
}
