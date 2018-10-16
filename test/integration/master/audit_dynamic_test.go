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

package master

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	auditregv1alpha1 "k8s.io/api/auditregistration/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/client-go/kubernetes"
	kubeapiservertesting "k8s.io/kubernetes/cmd/kube-apiserver/app/testing"
	"k8s.io/kubernetes/test/integration/framework"
	"k8s.io/kubernetes/test/utils"
)

var testEventList = TestEventList{}

type TestEventList struct {
	sync.RWMutex
	el auditinternal.EventList
}

// TestDynamicAudit ensures that v1alpha of the auditregistration api works
func TestDynamicAudit(t *testing.T) {

	// start api server
	result := kubeapiservertesting.StartTestServerOrDie(t, nil,
		[]string{
			"--audit-dynamic-configuration",
			"--feature-gates=DynamicAuditing=true",
			"--runtime-config=auditregistration.k8s.io/v1alpha1=true",
		},
		framework.SharedEtcd())
	defer result.TearDownFn()

	// start mock server consumer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("could not read request body: %v", err)
		}
		el := &auditinternal.EventList{}
		decoder := audit.Codecs.UniversalDecoder(auditv1.SchemeGroupVersion)
		if err := runtime.DecodeInto(decoder, body, el); err != nil {
			t.Fatalf("failed decoding buf: %b, apiVersion: %s", body, auditv1.SchemeGroupVersion)
		}
		defer r.Body.Close()

		testEventList.Lock()
		defer testEventList.Unlock()
		testEventList.el.Items = append(testEventList.el.Items, el.Items...)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	t.Logf("starting mock audit server on addr: %s", srv.URL)

	time.Sleep(1 * time.Second)

	kubeclient, err := kubernetes.NewForConfig(result.ClientConfig)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

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
					URL: &srv.URL,
				},
			},
		},
	}
	_, err = kubeclient.AuditregistrationV1alpha1().AuditSinks().Create(&sink)
	expectNoError(t, err, "failed to create audit sink")
	t.Log("created audit sink")

	// perform configmap ops
	configMapOperations(t, kubeclient)

	// check for corresponding events
	var missing []utils.AuditEvent
	checkFn := wait.ConditionFunc(func() (bool, error) {
		var err error
		testEventList.RLock()
		defer testEventList.RUnlock()
		missing, err = utils.CheckAuditList(testEventList.el, expectedEvents)
		if err != nil {
			t.Fatalf("problem checking audit list: %v", err)
		}
		if len(missing) > 0 {
			return false, nil
		}
		t.Logf("all events found")
		return true, nil
	})
	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1,
		Jitter:   0,
		Steps:    30,
	}
	err = wait.ExponentialBackoff(backoff, checkFn)
	if err != nil {
		t.Errorf("Failed to match all expected events, events %#v not found!", missing)
	}
}
