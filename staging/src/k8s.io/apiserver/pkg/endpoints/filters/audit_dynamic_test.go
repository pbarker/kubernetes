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

package filters

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/pborman/uuid"

	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type simpleDynamicResponseWriter struct{}

var _ http.ResponseWriter = &simpleDynamicResponseWriter{}

func (*simpleDynamicResponseWriter) WriteHeader(code int)         {}
func (*simpleDynamicResponseWriter) Write(bs []byte) (int, error) { return len(bs), nil }
func (*simpleDynamicResponseWriter) Header() http.Header          { return http.Header{} }

type fancyDynamicResponseWriter struct {
	simpleDynamicResponseWriter
}

func (*fancyDynamicResponseWriter) CloseNotify() <-chan bool { return nil }

func (*fancyDynamicResponseWriter) Flush() {}

func (*fancyDynamicResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

func TestConstructDynamicResponseWriter(t *testing.T) {
	actual := decorateDynamicResponseWriter(&simpleDynamicResponseWriter{}, nil, nil, nil)
	switch v := actual.(type) {
	case *auditDynamicResponseWriter:
	default:
		t.Errorf("Expected auditDynamicResponseWriter, got %v", reflect.TypeOf(v))
	}

	actual = decorateDynamicResponseWriter(&fancyDynamicResponseWriter{}, nil, nil, nil)
	switch v := actual.(type) {
	case *fancyDynamicResponseWriterDelegator:
	default:
		t.Errorf("Expected fancyDynamicResponseWriterDelegator, got %v", reflect.TypeOf(v))
	}
}

func TestDecorateDynamicResponseWriterChannel(t *testing.T) {
	attribs := authorizer.AttributesRecord{}
	policy := policy.DynamicPolicy{
		Level:  auditinternal.LevelRequestResponse,
		Stages: policy.AllStages,
	}
	sink := &fakeAuditSink{policy: policy}
	ev := &auditinternal.Event{Level: auditinternal.LevelRequestResponse}
	actual := decorateDynamicResponseWriter(&simpleDynamicResponseWriter{}, ev, attribs, sink)

	done := make(chan struct{})
	go func() {
		t.Log("Writing status code 42")
		actual.WriteHeader(42)
		t.Log("Finished writing status code 42")
		close(done)

		actual.Write([]byte("foo"))
	}()

	// sleep some time to give write the possibility to do wrong stuff
	time.Sleep(100 * time.Millisecond)

	t.Log("Waiting for event in the channel")
	ev1, err := sink.Pop(time.Second)
	if err != nil {
		t.Fatal("Timeout waiting for events")
	}
	t.Logf("Seen event with status %v", ev1.ResponseStatus)

	if !reflect.DeepEqual(ev, ev1) {
		t.Fatalf("ev1 and ev must be equal")
	}

	<-done
	t.Log("Seen the go routine finished")

	// write again
	_, err = actual.Write([]byte("foo"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDynamicAuditIDHttpHeader(t *testing.T) {
	for _, test := range []struct {
		desc           string
		requestHeader  string
		level          auditinternal.Level
		expectedHeader bool
	}{
		{
			"server generated header",
			"",
			auditinternal.LevelRequestResponse,
			true,
		},
		{
			"user provided header",
			uuid.NewRandom().String(),
			auditinternal.LevelRequestResponse,
			true,
		},
	} {
		policy := policy.DynamicPolicy{
			Level:  test.level,
			Stages: policy.AllStages,
		}
		sink := &fakeAuditSink{policy: policy}
		var handler http.Handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
		})
		handler = WithDynamicAudit(handler, sink, nil)

		req, _ := http.NewRequest("GET", "/api/v1/namespaces/default/pods", nil)
		req.RemoteAddr = "127.0.0.1"
		req = withTestContext(req, &user.DefaultInfo{Name: "admin"}, nil)
		if test.requestHeader != "" {
			req.Header.Add("Audit-ID", test.requestHeader)
		}

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		resp := w.Result()
		if test.expectedHeader {
			if resp.Header.Get("Audit-ID") == "" {
				t.Errorf("[%s] expected Audit-ID http header returned, but not returned", test.desc)
				continue
			}
			// if get Audit-ID returned, it should be the same same with the requested one
			if test.requestHeader != "" && resp.Header.Get("Audit-ID") != test.requestHeader {
				t.Errorf("[%s] returned audit http header is not the same with the requested http header, expected: %s, get %s", test.desc, test.requestHeader, resp.Header.Get("Audit-ID"))
			}
		} else {
			if resp.Header.Get("Audit-ID") != "" {
				t.Errorf("[%s] expected no Audit-ID http header returned, but got %s", test.desc, resp.Header.Get("Audit-ID"))
			}
		}
	}
}
