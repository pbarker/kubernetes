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
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/apiserver/pkg/endpoints/request"
)

// WithDynamicAudit decorates a http.Handler with audit logging information for all the
// requests coming to the server. Audit level is decided according to the downstream sink.
// Logs are emitted to the enforced audit sink to process events. If sink nil, no decoration takes place.
func WithDynamicAudit(handler http.Handler, sink audit.EnforcedSink, longRunningCheck request.LongRunningRequestCheck) http.Handler {
	if sink == nil {
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		attribs, err := GetAuthorizerAttributes(ctx)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to GetAuthorizerAttributes: %v", err))
			responsewriters.InternalError(w, req, errors.New("failed to get authorizer attributes"))
			return
		}

		// create event at maximum level
		ev, err := audit.NewEventFromRequest(req, auditinternal.LevelRequestResponse, attribs)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to complete audit event from request: %v", err))
			responsewriters.InternalError(w, req, errors.New("failed to create event from request"))
			return
		}

		req = req.WithContext(request.WithAuditEvent(ctx, ev))

		if ev == nil || ctx == nil {
			handler.ServeHTTP(w, req)
			return
		}

		ev.Stage = auditinternal.StageRequestReceived
		processAuditEnforcedEvent(sink, ev, attribs)

		// intercept the status code
		var longRunningSink audit.EnforcedSink
		if longRunningCheck != nil {
			ri, _ := request.RequestInfoFrom(ctx)
			if longRunningCheck(req, ri) {
				longRunningSink = sink
			}
		}
		respWriter := decorateDynamicResponseWriter(w, ev, attribs, longRunningSink)

		// send audit event when we leave this func, either via a panic or cleanly. In the case of long
		// running requests, this will be the second audit event.
		defer func() {
			if r := recover(); r != nil {
				defer panic(r)
				ev.Stage = auditinternal.StagePanic
				ev.ResponseStatus = &metav1.Status{
					Code:    http.StatusInternalServerError,
					Status:  metav1.StatusFailure,
					Reason:  metav1.StatusReasonInternalError,
					Message: fmt.Sprintf("APIServer panic'd: %v", r),
				}
				processAuditEnforcedEvent(sink, ev, attribs)
				return
			}

			// if no StageResponseStarted event was sent b/c neither a status code nor a body was sent, fake it here
			// But Audit-Id http header will only be sent when http.ResponseWriter.WriteHeader is called.
			fakedSuccessStatus := &metav1.Status{
				Code:    http.StatusOK,
				Status:  metav1.StatusSuccess,
				Message: "Connection closed early",
			}
			if ev.ResponseStatus == nil && longRunningSink != nil {
				ev.ResponseStatus = fakedSuccessStatus
				ev.Stage = auditinternal.StageResponseStarted
				processAuditEnforcedEvent(longRunningSink, ev, attribs)
			}

			ev.Stage = auditinternal.StageResponseComplete
			if ev.ResponseStatus == nil {
				ev.ResponseStatus = fakedSuccessStatus
			}
			processAuditEnforcedEvent(sink, ev, attribs)
		}()
		handler.ServeHTTP(respWriter, req)
	})
}

func processAuditEnforcedEvent(sink audit.EnforcedSink, ev *auditinternal.Event, attribs authorizer.Attributes) {

	if ev.Stage == auditinternal.StageRequestReceived {
		ev.StageTimestamp = metav1.NewMicroTime(ev.RequestReceivedTimestamp.Time)
	} else {
		ev.StageTimestamp = metav1.NewMicroTime(time.Now())
	}

	audit.ObserveEvent()
	ee := &audit.EnforcedEvent{
		Event:      ev,
		Attributes: attribs,
	}
	sink.ProcessEnforcedEvents(ee)
}

func decorateDynamicResponseWriter(
	responseWriter http.ResponseWriter,
	ev *auditinternal.Event,
	attribs authorizer.Attributes,
	sink audit.EnforcedSink,
) http.ResponseWriter {
	delegate := &auditDynamicResponseWriter{
		ResponseWriter: responseWriter,
		event:          ev,
		sink:           sink,
		attribs:        attribs,
	}

	// check if the ResponseWriter we're wrapping is the fancy one we need
	// or if the basic is sufficient
	_, cn := responseWriter.(http.CloseNotifier)
	_, fl := responseWriter.(http.Flusher)
	_, hj := responseWriter.(http.Hijacker)
	if cn && fl && hj {
		return &fancyDynamicResponseWriterDelegator{delegate}
	}
	return delegate
}

// auditDynamicResponseWriter intercepts WriteHeader, sets it in the event. If the sink is set, it will
// create immediately an event (for long running requests).
type auditDynamicResponseWriter struct {
	http.ResponseWriter
	event   *auditinternal.Event
	attribs authorizer.Attributes
	once    sync.Once
	sink    audit.EnforcedSink
}

func (a *auditDynamicResponseWriter) setHttpHeader() {
	a.ResponseWriter.Header().Set(auditinternal.HeaderAuditID, string(a.event.AuditID))
}

func (a *auditDynamicResponseWriter) processCode(code int) {
	a.once.Do(func() {
		if a.event.ResponseStatus == nil {
			a.event.ResponseStatus = &metav1.Status{}
		}
		a.event.ResponseStatus.Code = int32(code)
		a.event.Stage = auditinternal.StageResponseStarted

		if a.sink != nil {
			processAuditEnforcedEvent(a.sink, a.event, a.attribs)
		}
	})
}

func (a *auditDynamicResponseWriter) Write(bs []byte) (int, error) {
	// the Go library calls WriteHeader internally if no code was written yet. But this will go unnoticed for us
	a.processCode(http.StatusOK)
	a.setHttpHeader()
	return a.ResponseWriter.Write(bs)
}

func (a *auditDynamicResponseWriter) WriteHeader(code int) {
	a.processCode(code)
	a.setHttpHeader()
	a.ResponseWriter.WriteHeader(code)
}

// fancyDynamicResponseWriterDelegator implements http.CloseNotifier, http.Flusher and
// http.Hijacker which are needed to make certain http operation (e.g. watch, rsh, etc)
// working.
type fancyDynamicResponseWriterDelegator struct {
	*auditDynamicResponseWriter
}

func (f *fancyDynamicResponseWriterDelegator) CloseNotify() <-chan bool {
	return f.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (f *fancyDynamicResponseWriterDelegator) Flush() {
	f.ResponseWriter.(http.Flusher).Flush()
}

func (f *fancyDynamicResponseWriterDelegator) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// fake a response status before protocol switch happens
	f.processCode(http.StatusSwitchingProtocols)

	// This will be ignored if WriteHeader() function has already been called.
	// It's not guaranteed Audit-ID http header is sent for all requests.
	// For example, when user run "kubectl exec", apiserver uses a proxy handler
	// to deal with the request, users can only get http headers returned by kubelet node.
	f.setHttpHeader()

	return f.ResponseWriter.(http.Hijacker).Hijack()
}

var _ http.CloseNotifier = &fancyDynamicResponseWriterDelegator{}
var _ http.Flusher = &fancyDynamicResponseWriterDelegator{}
var _ http.Hijacker = &fancyDynamicResponseWriterDelegator{}
