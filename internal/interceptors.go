// Copyright (c) 2020 Temporal Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package internal

import (
	"github.com/uber-go/tally"
	"time"
)
import "go.uber.org/zap"

// WorkflowInterceptorFactory is used to create a single link in the interceptor chain
type WorkflowInterceptorFactory interface {
	// NewInterceptor creates an interceptor instance. The created instance must delegate every call to
	// the next parameter for workflow code function correctly.
	NewInterceptor(next WorkflowInterceptor) WorkflowInterceptor
}

// WorkflowInterceptor is an interface that can be implemented to intercept calls done by the workflow code.
type WorkflowInterceptor interface {
	ExecuteActivity(ctx Context, activity interface{}, args ...interface{}) Future
	ExecuteLocalActivity(ctx Context, activity interface{}, args ...interface{}) Future
	ExecuteChildWorkflow(ctx Context, childWorkflow interface{}, args ...interface{}) ChildWorkflowFuture
	GetWorkflowInfo(ctx Context) *WorkflowInfo
	GetLogger(ctx Context) *zap.Logger
	GetMetricsScope(ctx Context) tally.Scope
	Now(ctx Context) time.Time
	NewTimer(ctx Context, d time.Duration) Future
	Sleep(ctx Context, d time.Duration) (err error)
	RequestCancelExternalWorkflow(ctx Context, workflowID, runID string) Future
	SignalExternalWorkflow(ctx Context, workflowID, runID, signalName string, arg interface{}) Future
	UpsertSearchAttributes(ctx Context, attributes map[string]interface{}) error
	GetSignalChannel(ctx Context, signalName string) Channel
	SideEffect(ctx Context, f func(ctx Context) interface{}) Value
	MutableSideEffect(ctx Context, id string, f func(ctx Context) interface{}, equals func(a, b interface{}) bool) Value
	GetVersion(ctx Context, changeID string, minSupported, maxSupported Version) Version
	SetQueryHandler(ctx Context, queryType string, handler interface{}) error
	IsReplaying(ctx Context) bool
	HasLastCompletionResult(ctx Context) bool
	GetLastCompletionResult(ctx Context, d ...interface{}) error
}

var _ WorkflowInterceptor = (*WorkflowInterceptorBase)(nil)

type WorkflowInterceptorBase struct {
	next WorkflowInterceptor
}

func (t *WorkflowInterceptorBase) ExecuteActivity(ctx Context, activity interface{}, args ...interface{}) Future {
	return t.next.ExecuteActivity(ctx, activity, args...)
}

func (t *WorkflowInterceptorBase) ExecuteLocalActivity(ctx Context, activity interface{}, args ...interface{}) Future {
	return t.next.ExecuteLocalActivity(ctx, activity, args...)
}

func (t *WorkflowInterceptorBase) ExecuteChildWorkflow(ctx Context, childWorkflow interface{}, args ...interface{}) ChildWorkflowFuture {
	return t.next.ExecuteChildWorkflow(ctx, childWorkflow, args...)
}

func (t *WorkflowInterceptorBase) GetWorkflowInfo(ctx Context) *WorkflowInfo {
	return t.next.GetWorkflowInfo(ctx)
}

func (t *WorkflowInterceptorBase) GetLogger(ctx Context) *zap.Logger {
	return t.next.GetLogger(ctx)
}

func (t *WorkflowInterceptorBase) GetMetricsScope(ctx Context) tally.Scope {
	return t.next.GetMetricsScope(ctx)
}

func (t *WorkflowInterceptorBase) Now(ctx Context) time.Time {
	return t.next.Now(ctx)
}

func (t *WorkflowInterceptorBase) NewTimer(ctx Context, d time.Duration) Future {
	return t.next.NewTimer(ctx, d)
}

func (t *WorkflowInterceptorBase) Sleep(ctx Context, d time.Duration) (err error) {
	return t.next.Sleep(ctx, d)
}

func (t *WorkflowInterceptorBase) RequestCancelExternalWorkflow(ctx Context, workflowID, runID string) Future {
	return t.next.RequestCancelExternalWorkflow(ctx, workflowID, runID)
}

func (t *WorkflowInterceptorBase) SignalExternalWorkflow(ctx Context, workflowID, runID, signalName string, arg interface{}) Future {
	return t.next.SignalExternalWorkflow(ctx, workflowID, runID, signalName, arg)
}

func (t *WorkflowInterceptorBase) UpsertSearchAttributes(ctx Context, attributes map[string]interface{}) error {
	return t.next.UpsertSearchAttributes(ctx, attributes)
}

func (t *WorkflowInterceptorBase) GetSignalChannel(ctx Context, signalName string) Channel {
	return t.next.GetSignalChannel(ctx, signalName)
}

func (t *WorkflowInterceptorBase) SideEffect(ctx Context, f func(ctx Context) interface{}) Value {
	return t.next.SideEffect(ctx, f)
}

func (t *WorkflowInterceptorBase) MutableSideEffect(ctx Context, id string, f func(ctx Context) interface{}, equals func(a, b interface{}) bool) Value {
	return t.next.MutableSideEffect(ctx, id, f, equals)
}

func (t *WorkflowInterceptorBase) GetVersion(ctx Context, changeID string, minSupported, maxSupported Version) Version {
	return t.next.GetVersion(ctx, changeID, minSupported, maxSupported)
}

func (t *WorkflowInterceptorBase) SetQueryHandler(ctx Context, queryType string, handler interface{}) error {
	return t.next.SetQueryHandler(ctx, queryType, handler)
}

func (t *WorkflowInterceptorBase) IsReplaying(ctx Context) bool {
	return t.next.IsReplaying(ctx)
}

func (t *WorkflowInterceptorBase) HasLastCompletionResult(ctx Context) bool {
	return t.next.HasLastCompletionResult(ctx)
}

func (t *WorkflowInterceptorBase) GetLastCompletionResult(ctx Context, d ...interface{}) error {
	return t.next.GetLastCompletionResult(ctx, d...)
}
