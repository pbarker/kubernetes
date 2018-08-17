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
	if b.InitialBackoff != nil && *b.InitialBackoff <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("initialBackoff"), b.InitialBackoff, "initial backoff must be a positive number"))
	}
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
	rulePath := field.NewPath("rules")
	for i, rule := range policy.Rules {
		allErrs = append(allErrs, validatePolicyRule(rule, rulePath.Index(i))...)
	}
	return allErrs
}

func validatePolicyRule(rule auditregistration.PolicyRule, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateLevel(rule.Level, fldPath.Child("level"))...)
	allErrs = append(allErrs, validateNonResourceURLs(rule.NonResourceURLs, fldPath.Child("nonResourceURLs"))...)
	allErrs = append(allErrs, validateResources(rule.Resources, fldPath.Child("resources"))...)
	allErrs = append(allErrs, validateOmitStages(rule.OmitStages, fldPath.Child("omitStages"))...)

	if len(rule.NonResourceURLs) > 0 {
		if len(rule.Resources) > 0 || len(rule.Namespaces) > 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("nonResourceURLs"), rule.NonResourceURLs, "rules cannot apply to both regular resources and non-resource URLs"))
		}
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

func validateNonResourceURLs(urls []string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for i, url := range urls {
		if url == "*" {
			continue
		}

		if !strings.HasPrefix(url, "/") {
			allErrs = append(allErrs, field.Invalid(fldPath.Index(i), url, "non-resource URL rules must begin with a '/' character"))
		}

		if url != "" && strings.ContainsRune(url[:len(url)-1], '*') {
			allErrs = append(allErrs, field.Invalid(fldPath.Index(i), url, "non-resource URL wildcards '*' must be the final character of the rule"))
		}
	}
	return allErrs
}

func validateResources(groupResources []auditregistration.GroupResources, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for _, groupResource := range groupResources {
		// The empty string represents the core API group.
		if len(groupResource.Group) != 0 {
			// Group names must be lower case and be valid DNS subdomains.
			// reference: https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md
			// an error is returned for group name like rbac.authorization.k8s.io/v1beta1
			// rbac.authorization.k8s.io is the valid one
			if msgs := genericvalidation.NameIsDNSSubdomain(groupResource.Group, false); len(msgs) != 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("group"), groupResource.Group, strings.Join(msgs, ",")))
			}
		}

		if len(groupResource.ResourceNames) > 0 && len(groupResource.Resources) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("resourceNames"), groupResource.ResourceNames, "using resourceNames requires at least one resource"))
		}
	}
	return allErrs
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
