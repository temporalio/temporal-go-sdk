// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
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

// All code in this file is private to the package.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/time/rate"

	commonpb "go.temporal.io/temporal-proto/common/v1"

	"go.temporal.io/temporal/internal/common/backoff"
	"go.temporal.io/temporal/internal/common/metrics"
)

const (
	retryPollOperationInitialInterval = 20 * time.Millisecond
	retryPollOperationMaxInterval     = 10 * time.Second
)

var (
	pollOperationRetryPolicy = createPollRetryPolicy()
)

var errStop = errors.New("worker stopping")

type (
	// ResultHandler that returns result
	ResultHandler func(result *commonpb.Payloads, err error)
	// LocalActivityResultHandler that returns local activity result
	LocalActivityResultHandler func(lar *LocalActivityResultWrapper)

	// LocalActivityResultWrapper contains result of a local activity
	LocalActivityResultWrapper struct {
		Err     error
		Result  *commonpb.Payloads
		Attempt int32
		Backoff time.Duration
	}

	// WorkflowEnvironment Represents the environment for workflow/decider.
	// Should only be used within the scope of workflow definition
	WorkflowEnvironment interface {
		AsyncActivityClient
		LocalActivityClient
		WorkflowTimerClient
		SideEffect(f func() (*commonpb.Payloads, error), callback ResultHandler)
		GetVersion(changeID string, minSupported, maxSupported Version) Version
		WorkflowInfo() *WorkflowInfo
		Complete(result *commonpb.Payloads, err error)
		RegisterCancelHandler(handler func())
		RequestCancelChildWorkflow(namespace, workflowID string)
		RequestCancelExternalWorkflow(namespace, workflowID, runID string, callback ResultHandler)
		ExecuteChildWorkflow(params ExecuteWorkflowParams, callback ResultHandler, startedHandler func(r WorkflowExecution, e error))
		GetLogger() *zap.Logger
		GetMetricsScope() tally.Scope
		// Must be called before WorkflowDefinition.Execute returns
		RegisterSignalHandler(handler func(name string, input *commonpb.Payloads))
		SignalExternalWorkflow(namespace, workflowID, runID, signalName string, input *commonpb.Payloads, arg interface{}, childWorkflowOnly bool, callback ResultHandler)
		RegisterQueryHandler(handler func(queryType string, queryArgs *commonpb.Payloads) (*commonpb.Payloads, error))
		IsReplaying() bool
		MutableSideEffect(id string, f func() interface{}, equals func(a, b interface{}) bool) Value
		GetDataConverter() DataConverter
		AddSession(sessionInfo *SessionInfo)
		RemoveSession(sessionID string)
		GetContextPropagators() []ContextPropagator
		UpsertSearchAttributes(attributes map[string]interface{}) error
		GetRegistry() *registry
	}

	// WorkflowDefinitionFactory factory for creating WorkflowDefinition instances.
	WorkflowDefinitionFactory interface {
		// NewWorkflowDefinition must return a new instance of WorkflowDefinition on each call
		NewWorkflowDefinition() WorkflowDefinition
	}

	// WorkflowDefinition wraps the code that can execute a workflow.
	WorkflowDefinition interface {
		// Implementation must be asynchronous
		Execute(env WorkflowEnvironment, header *commonpb.Header, input *commonpb.Payloads)
		// Called for each non timed out startDecision event.
		// Executed after all history events since the previous decision are applied to WorkflowDefinition
		// Application level code must be executed from this function only.
		// Execute call as well as callbacks called from WorkflowEnvironment functions can only schedule callbacks
		// which can be executed from OnDecisionTaskStarted()
		OnDecisionTaskStarted()
		StackTrace() string // Stack trace of all coroutines owned by the Dispatcher instance
		Close()
	}

	// baseWorkerOptions options to configure base worker.
	baseWorkerOptions struct {
		pollerCount       int
		pollerRate        int
		maxConcurrentTask int
		maxTaskPerSecond  float64
		taskWorker        taskPoller
		identity          string
		workerType        string
		stopTimeout       time.Duration
		userContextCancel context.CancelFunc
	}

	// baseWorker that wraps worker activities.
	baseWorker struct {
		options              baseWorkerOptions
		isWorkerStarted      bool
		stopCh               chan struct{}  // Channel used to stop the go routines.
		stopWG               sync.WaitGroup // The WaitGroup for stopping existing routines.
		pollLimiter          *rate.Limiter
		taskLimiter          *rate.Limiter
		limiterContext       context.Context
		limiterContextCancel func()
		retrier              *backoff.ConcurrentRetrier // Service errors back off retrier
		logger               *zap.Logger
		metricsScope         tally.Scope

		pollerRequestCh    chan struct{}
		taskQueueCh        chan interface{}
		sessionTokenBucket *sessionTokenBucket
	}

	polledTask struct {
		task interface{}
	}
)

func createPollRetryPolicy() backoff.RetryPolicy {
	policy := backoff.NewExponentialRetryPolicy(retryPollOperationInitialInterval)
	policy.SetMaximumInterval(retryPollOperationMaxInterval)

	// NOTE: We don't use expiration interval since we don't use retries from retrier class.
	// We use it to calculate next backoff. We have additional layer that is built on poller
	// in the worker layer for to add some middleware for any poll retry that includes
	// (a) rate limiting across pollers (b) back-off across pollers when server is busy
	policy.SetExpirationInterval(backoff.NoInterval) // We don't ever expire
	return policy
}

func newBaseWorker(options baseWorkerOptions, logger *zap.Logger, metricsScope tally.Scope, sessionTokenBucket *sessionTokenBucket) *baseWorker {
	ctx, cancel := context.WithCancel(context.Background())
	bw := &baseWorker{
		options:         options,
		stopCh:          make(chan struct{}),
		taskLimiter:     rate.NewLimiter(rate.Limit(options.maxTaskPerSecond), 1),
		retrier:         backoff.NewConcurrentRetrier(pollOperationRetryPolicy),
		logger:          logger.With(zapcore.Field{Key: tagWorkerType, Type: zapcore.StringType, String: options.workerType}),
		metricsScope:    tagScope(metricsScope, tagWorkerType, options.workerType),
		pollerRequestCh: make(chan struct{}, options.maxConcurrentTask),
		taskQueueCh:     make(chan interface{}), // no buffer, so poller only able to poll new task after previous is dispatched.

		limiterContext:       ctx,
		limiterContextCancel: cancel,
		sessionTokenBucket:   sessionTokenBucket,
	}
	if options.pollerRate > 0 {
		bw.pollLimiter = rate.NewLimiter(rate.Limit(options.pollerRate), 1)
	}

	return bw
}

// Start starts a fixed set of routines to do the work.
func (bw *baseWorker) Start() {
	if bw.isWorkerStarted {
		return
	}

	bw.metricsScope.Counter(metrics.WorkerStartCounter).Inc(1)

	for i := 0; i < bw.options.pollerCount; i++ {
		bw.stopWG.Add(1)
		go bw.runPoller()
	}

	bw.stopWG.Add(1)
	go bw.runTaskDispatcher()

	bw.isWorkerStarted = true
	traceLog(func() {
		bw.logger.Info("Started Worker",
			zap.Int("PollerCount", bw.options.pollerCount),
			zap.Int("MaxConcurrentTask", bw.options.maxConcurrentTask),
			zap.Float64("MaxTaskPerSecond", bw.options.maxTaskPerSecond),
		)
	})
}

func (bw *baseWorker) isStop() bool {
	select {
	case <-bw.stopCh:
		return true
	default:
		return false
	}
}

func (bw *baseWorker) runPoller() {
	defer bw.stopWG.Done()
	bw.metricsScope.Counter(metrics.PollerStartCounter).Inc(1)

	for {
		select {
		case <-bw.stopCh:
			return
		case <-bw.pollerRequestCh:
			if bw.sessionTokenBucket != nil {
				bw.sessionTokenBucket.waitForAvailableToken()
			}
			bw.pollTask()
		}
	}
}

func (bw *baseWorker) runTaskDispatcher() {
	defer bw.stopWG.Done()

	for i := 0; i < bw.options.maxConcurrentTask; i++ {
		bw.pollerRequestCh <- struct{}{}
	}

	for {
		// wait for new task or worker stop
		select {
		case <-bw.stopCh:
			return
		case task := <-bw.taskQueueCh:
			// for non-polled-task (local activity result as task), we don't need to rate limit
			_, isPolledTask := task.(*polledTask)
			if isPolledTask && bw.taskLimiter.Wait(bw.limiterContext) != nil {
				if bw.isStop() {
					return
				}
			}
			bw.stopWG.Add(1)
			go bw.processTask(task)
		}
	}
}

func (bw *baseWorker) pollTask() {
	var err error
	var task interface{}
	bw.retrier.Throttle()
	if bw.pollLimiter == nil || bw.pollLimiter.Wait(bw.limiterContext) == nil {
		task, err = bw.options.taskWorker.PollTask()
		if err != nil && enableVerboseLogging {
			bw.logger.Debug("Failed to poll for task.", zap.Error(err))
		}
		if err != nil && isServiceTransientError(err) {
			bw.retrier.Failed()
		} else {
			bw.retrier.Succeeded()
		}
	}

	if task != nil {
		select {
		case bw.taskQueueCh <- &polledTask{task}:
		case <-bw.stopCh:
		}
	} else {
		bw.pollerRequestCh <- struct{}{} // poll failed, trigger a new poll
	}
}

func (bw *baseWorker) processTask(task interface{}) {
	defer bw.stopWG.Done()
	// If the task is from poller, after processing it we would need to request a new poll. Otherwise, the task is from
	// local activity worker, we don't need a new poll from server.
	polledTask, isPolledTask := task.(*polledTask)
	if isPolledTask {
		task = polledTask.task
	}
	defer func() {
		if p := recover(); p != nil {
			bw.metricsScope.Counter(metrics.WorkerPanicCounter).Inc(1)
			topLine := fmt.Sprintf("base worker for %s [panic]:", bw.options.workerType)
			st := getStackTraceRaw(topLine, 7, 0)
			bw.logger.Error("Unhandled panic.",
				zap.String("PanicError", fmt.Sprintf("%v", p)),
				zap.String("PanicStack", st))
		}

		if isPolledTask {
			bw.pollerRequestCh <- struct{}{}
		}
	}()
	err := bw.options.taskWorker.ProcessTask(task)
	if err != nil {
		if isClientSideError(err) {
			bw.logger.Info("Task processing failed with client side error", zap.Error(err))
		} else {
			bw.logger.Info("Task processing failed with error", zap.Error(err))
		}
	}
}

func (bw *baseWorker) Run() {
	bw.Start()
	d := <-getKillSignal()
	traceLog(func() {
		bw.logger.Info("Worker has been killed", zap.String("Signal", d.String()))
	})
	bw.Stop()
}

// Stop is a blocking call and cleans up all the resources associated with worker.
func (bw *baseWorker) Stop() {
	if !bw.isWorkerStarted {
		return
	}
	close(bw.stopCh)
	bw.limiterContextCancel()

	if success := awaitWaitGroup(&bw.stopWG, bw.options.stopTimeout); !success {
		traceLog(func() {
			bw.logger.Info("Worker graceful stop timed out.", zap.Duration("Stop timeout", bw.options.stopTimeout))
		})
	}

	// Close context
	if bw.options.userContextCancel != nil {
		bw.options.userContextCancel()
	}
}
