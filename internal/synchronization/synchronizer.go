// Copyright 2023 f5 Inc. All rights reserved.
// Use of this source code is governed by the Apache
// license that can be found in the LICENSE file.

package synchronization

import (
	"fmt"
	"github.com/nginxinc/kubernetes-nginx-ingress/internal/communication"
	"github.com/nginxinc/kubernetes-nginx-ingress/internal/config"
	"github.com/nginxinc/kubernetes-nginx-ingress/internal/core"
	nginxClient "github.com/nginxinc/nginx-plus-go-client/client"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"time"
)

const RateLimiterBase = time.Second * 2
const RateLimiterMax = time.Second * 60
const RetryCount = 5
const Threads = 1
const SynchronizerQueueName = `nec-synchronizer`

type Synchronizer struct {
	NginxPlusClient *nginxClient.NginxClient
	eventQueue      workqueue.RateLimitingInterface
}

func NewSynchronizer() (*Synchronizer, error) {
	synchronizer := Synchronizer{}

	return &synchronizer, nil
}

func (s *Synchronizer) AddRateLimitedEvent(event *core.Event) {
	logrus.Debugf(`Synchronizer::AddRateLimitedEvent: %#v`, event)
	s.eventQueue.AddRateLimited(event)
}

func (s *Synchronizer) Initialize() error {
	var err error
	settings, err := config.NewSettings()
	if err != nil {
		return fmt.Errorf(`error loading configuration: %v`, err)
	}

	httpClient, err := communication.NewHttpClient()
	if err != nil {
		return fmt.Errorf(`error creating HTTP client: %v`, err)
	}

	s.NginxPlusClient, err = nginxClient.NewNginxClient(httpClient, settings.NginxPlusHost)
	if err != nil {
		return fmt.Errorf(`error creating Nginx Plus client: %v`, err)
	}

	rateLimiter := workqueue.NewItemExponentialFailureRateLimiter(RateLimiterBase, RateLimiterMax)

	s.eventQueue = workqueue.NewNamedRateLimitingQueue(rateLimiter, SynchronizerQueueName)

	return nil
}

func (s *Synchronizer) Run(stopCh <-chan struct{}) {
	logrus.Debug(`Synchronizer::Run`)

	for i := 0; i < Threads; i++ {
		go wait.Until(s.worker, 0, stopCh)
	}

	<-stopCh
}

func (s *Synchronizer) ShutDown() {
	logrus.Debugf(`Synchronizer::ShutDown`)
	s.eventQueue.ShutDownWithDrain()
}

func (s *Synchronizer) handleEvent(event *core.Event) error {
	logrus.Infof(`Synchronizer::handleEvent: %#v`, event)

	_, _, _, err := s.NginxPlusClient.UpdateHTTPServers("", event.NginxUpstreams)
	if err != nil {
		return fmt.Errorf(`error occurred updating the nginx+ host: %w`, err)
	}

	return nil
}

func (s *Synchronizer) handleNextEvent() bool {
	logrus.Debug(`Synchronizer::handleNextEvent`)
	evt, quit := s.eventQueue.Get()
	if quit {
		return false
	}

	defer s.eventQueue.Done(evt)

	event := evt.(*core.Event)
	s.withRetry(s.handleEvent(event), event)

	return true
}

func (s *Synchronizer) worker() {
	logrus.Debug(`Synchronizer::worker`)
	for s.handleNextEvent() {
		// TODO: Add Telemetry
	}
}

func (s *Synchronizer) withRetry(err error, event *core.Event) {
	logrus.Debug("Synchronizer::withRetry")
	if err != nil {
		// TODO: Add Telemetry
		if s.eventQueue.NumRequeues(event) < RetryCount { // TODO: Make this configurable
			s.eventQueue.AddRateLimited(event)
			logrus.Infof(`Synchronizer::withRetry: requeued event: %#v; error: %v`, event, err)
		} else {
			s.eventQueue.Forget(event)
			logrus.Warnf(`Synchronizer::withRetry: event %#v has been dropped due to too many retries`, event)
		}
	} // TODO: Add error logging
}
