/*
Copyright 2017 The Kubernetes Authors.

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

package auth

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"

	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/utils"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

var _ = SIGDescribe("[Feature:DynamicAudit]", func() {
	f := framework.NewDefaultFramework("audit")
	// BeforeEach(func() {
	// 	framework.SkipUnlessProviderIs("gce")
	// })

	// TODO: Get rid of [DisabledForLargeClusters] when feature request #53455 is ready.
	It("should dynamically audit API calls [DisabledForLargeClusters]", func() {
		namespace := f.Namespace.Name

		By("Creating a kubernetes client that impersonates an unauthorized anonymous user")
		config, err := framework.LoadConfig()
		framework.ExpectNoError(err, "failed to fetch config")

		config.Impersonate = restclient.ImpersonationConfig{
			UserName: "system:anonymous",
			Groups:   []string{"system:unauthenticated"},
		}
		anonymousClient, err := clientset.NewForConfig(config)
		framework.ExpectNoError(err, "failed to create the anonymous client")

		_, err = f.ClientSet.CoreV1().Namespaces().Create(&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "audit",
			},
		})
		framework.ExpectNoError(err, "failed to create namespace")

		_, err = f.ClientSet.CoreV1().Pods(namespace).Create(&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "audit-proxy",
				Labels: map[string]string{
					"app": "audit",
				},
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					apiv1.Container{
						Name: "proxy",
						// Image: imageutils.GetE2EImage(imageutils.AuditProxy),
						Image: "grillz/audit-proxy:1.0",
						Ports: []apiv1.ContainerPort{
							apiv1.ContainerPort{
								ContainerPort: 8080,
							},
						},
					},
				},
			},
		})
		framework.ExpectNoError(err, "failed to create proxy pod")

		_, err = f.ClientSet.CoreV1().Services(namespace).Create(&apiv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "audit",
			},
			Spec: apiv1.ServiceSpec{
				Ports: []apiv1.ServicePort{
					apiv1.ServicePort{
						Port:       80,
						TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
					},
				},
				Selector: map[string]string{
					"app": "audit",
				},
			},
		})
		framework.ExpectNoError(err, "failed to create proxy service")

		config, err = framework.LoadConfig()
		framework.ExpectNoError(err, "failed to load config")

		var podIP string
		// get pod ip
		err = wait.Poll(100*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			p, err := f.ClientSet.CoreV1().Pods(namespace).Get("audit-proxy", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, err
			}
			podIP = p.Status.PodIP
			if podIP == "" {
				return false, nil
			}
			return true, nil
		})
		framework.ExpectNoError(err, "could not look up pod ip")

		podURL := fmt.Sprintf("http://%s:8080", podIP)
		// create audit sink
		sink := auditregv1alpha1.AuditSink{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: auditregv1alpha1.AuditSinkSpec{
				Policy: auditregv1alpha1.Policy{
					Level: auditregv1alpha1.LevelRequestResponse,
					Stages: []auditregv1alpha1.Stage{
						auditregv1alpha1.StageRequestReceived,
						auditregv1alpha1.StageResponseStarted,
						auditregv1alpha1.StageResponseComplete,
						auditregv1alpha1.StagePanic,
					},
				},
				Webhook: auditregv1alpha1.Webhook{
					ClientConfig: auditregv1alpha1.WebhookClientConfig{
						URL: &podURL,
					},
				},
			},
		}

		_, err = f.ClientSet.AuditregistrationV1alpha1().AuditSinks().Create(&sink)
		framework.ExpectNoError(err, "failed to create audit sink")
		framework.Logf("created audit sink")
		fmt.Println("created audit sink")

		// check that we are receiving logs in the proxy
		err = wait.Poll(100*time.Millisecond, 10*time.Second, func() (done bool, err error) {
			logs, err := framework.GetPodLogs(f.ClientSet, namespace, "audit-proxy", "proxy")
			if err != nil {
				fmt.Println("could not get pod logs: ", err)
				return false, nil
			}
			if logs == "" {
				return false, nil
			}
			return true, nil
		})
		framework.ExpectNoError(err, "failed to get logs from sink")

		auditTestUser = "kubernetes-admin"
		testCases := []struct {
			action func()
			events []utils.AuditEvent
		}{
			// Create, get, update, patch, delete, list, watch pods.
			{
				func() {
					pod := &apiv1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "audit-pod",
						},
						Spec: apiv1.PodSpec{
							Containers: []apiv1.Container{{
								Name:  "pause",
								Image: imageutils.GetPauseImageName(),
							}},
						},
					}
					updatePod := func(pod *apiv1.Pod) {}

					f.PodClient().CreateSync(pod)

					_, err := f.PodClient().Get(pod.Name, metav1.GetOptions{})
					framework.ExpectNoError(err, "failed to get audit-pod")

					podChan, err := f.PodClient().Watch(watchOptions)
					framework.ExpectNoError(err, "failed to create watch for pods")
					for range podChan.ResultChan() {
					}

					f.PodClient().Update(pod.Name, updatePod)

					_, err = f.PodClient().List(metav1.ListOptions{})
					framework.ExpectNoError(err, "failed to list pods")

					_, err = f.PodClient().Patch(pod.Name, types.JSONPatchType, patch)
					framework.ExpectNoError(err, "failed to patch pod")

					f.PodClient().DeleteSync(pod.Name, &metav1.DeleteOptions{}, framework.DefaultPodDeletionTimeout)
				},
				[]utils.AuditEvent{
					{
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods", namespace),
						Verb:              "create",
						Code:              201,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     true,
						ResponseObject:    true,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods/audit-pod", namespace),
						Verb:              "get",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     false,
						ResponseObject:    true,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods", namespace),
						Verb:              "list",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     false,
						ResponseObject:    true,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseStarted,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods?timeout=%ds&timeoutSeconds=%d&watch=true", namespace, watchTestTimeout, watchTestTimeout),
						Verb:              "watch",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     false,
						ResponseObject:    false,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods?timeout=%ds&timeoutSeconds=%d&watch=true", namespace, watchTestTimeout, watchTestTimeout),
						Verb:              "watch",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     false,
						ResponseObject:    false,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods/audit-pod", namespace),
						Verb:              "update",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     true,
						ResponseObject:    true,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods/audit-pod", namespace),
						Verb:              "patch",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     true,
						ResponseObject:    true,
						AuthorizeDecision: "allow",
					}, {
						Level:             auditinternal.LevelRequestResponse,
						Stage:             auditinternal.StageResponseComplete,
						RequestURI:        fmt.Sprintf("/api/v1/namespaces/%s/pods/audit-pod", namespace),
						Verb:              "delete",
						Code:              200,
						User:              auditTestUser,
						Resource:          "pods",
						Namespace:         namespace,
						RequestObject:     true,
						ResponseObject:    true,
						AuthorizeDecision: "allow",
					},
				},
			},
		}

		// test authorizer annotations, RBAC is required.
		annotationTestCases := []struct {
			action func()
			events []utils.AuditEvent
		}{

			// get a pod with unauthorized user
			{
				func() {
					_, err := anonymousClient.CoreV1().Pods(namespace).Get("another-audit-pod", metav1.GetOptions{})
					expectForbidden(err)
				},
				[]utils.AuditEvent{
					{
						Level:              auditinternal.LevelRequestResponse,
						Stage:              auditinternal.StageResponseComplete,
						RequestURI:         fmt.Sprintf("/api/v1/namespaces/%s/pods/another-audit-pod", namespace),
						Verb:               "get",
						Code:               403,
						User:               auditTestUser,
						ImpersonatedUser:   "system:anonymous",
						ImpersonatedGroups: "system:unauthenticated",
						Resource:           "pods",
						Namespace:          namespace,
						RequestObject:      false,
						ResponseObject:     true,
						AuthorizeDecision:  "forbid",
					},
				},
			},
		}

		if framework.IsRBACEnabled(f) {
			testCases = append(testCases, annotationTestCases...)
		}
		expectedEvents := []utils.AuditEvent{}
		for _, t := range testCases {
			t.action()
			expectedEvents = append(expectedEvents, t.events...)
		}

		// The default flush timeout is 30 seconds, therefore it should be enough to retry once
		// to find all expected events. However, we're waiting for 5 minutes to avoid flakes.
		pollingInterval := 30 * time.Second
		pollingTimeout := 5 * time.Minute
		err = wait.Poll(pollingInterval, pollingTimeout, func() (bool, error) {
			// Fetch the logs
			logs, err := framework.GetPodLogs(f.ClientSet, namespace, "audit-proxy", "proxy")
			if err != nil {
				return false, err
			}
			reader := strings.NewReader(logs)
			missing, err := utils.CheckAuditLines(reader, expectedEvents, auditv1.SchemeGroupVersion)
			if err != nil {
				framework.Logf("Failed to observe audit events: %v", err)
			} else if len(missing) > 0 {
				framework.Logf("Events %#v not found!", missing)
			}
			return len(missing) == 0, nil
		})
		framework.ExpectNoError(err, "after %v failed to observe audit events", pollingTimeout)
		err = f.ClientSet.AuditregistrationV1alpha1().AuditSinks().Delete("test", &metav1.DeleteOptions{})
		framework.ExpectNoError(err, "could not delete audit configuration")
	})
})
