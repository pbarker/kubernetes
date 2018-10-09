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

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"

	lru "github.com/hashicorp/golang-lru"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	webhookerrors "k8s.io/apiserver/pkg/admission/plugin/webhook/errors"
	"k8s.io/client-go/rest"
)

const (
	defaultCacheSize = 200
)

var (
	// ErrNeedServiceOrURL error when neither url or service is given
	ErrNeedServiceOrURL = errors.New("webhook configuration must have either service or URL")
)

// ClientManager builds REST clients to talk to webhooks. It caches the clients
// to avoid duplicate creation.
type ClientManager struct {
	authInfoResolver     AuthenticationInfoResolver
	serviceResolver      ServiceResolver
	negotiatedSerializer runtime.NegotiatedSerializer
	cache                *lru.Cache
}

// NewClientManager creates a clientManager.
func NewClientManager(negotiatedSerializer runtime.NegotiatedSerializer) (ClientManager, error) {
	cache, err := lru.New(defaultCacheSize)
	if err != nil {
		return ClientManager{}, err
	}
	return ClientManager{
		cache:                cache,
		negotiatedSerializer: negotiatedSerializer,
	}, nil
}

// SetAuthenticationInfoResolverWrapper sets the
// AuthenticationInfoResolverWrapper.
func (cm *ClientManager) SetAuthenticationInfoResolverWrapper(wrapper AuthenticationInfoResolverWrapper) {
	if wrapper != nil {
		cm.authInfoResolver = wrapper(cm.authInfoResolver)
	}
}

// SetAuthenticationInfoResolver sets the AuthenticationInfoResolver.
func (cm *ClientManager) SetAuthenticationInfoResolver(resolver AuthenticationInfoResolver) {
	cm.authInfoResolver = resolver
}

// SetServiceResolver sets the ServiceResolver.
func (cm *ClientManager) SetServiceResolver(sr ServiceResolver) {
	if sr != nil {
		cm.serviceResolver = sr
	}
}

// Validate checks if ClientManager is properly set up.
func (cm *ClientManager) Validate() error {
	var errs []error
	if cm.negotiatedSerializer == nil {
		errs = append(errs, fmt.Errorf("the clientManager requires a negotiatedSerializer"))
	}
	if cm.serviceResolver == nil {
		errs = append(errs, fmt.Errorf("the clientManager requires a serviceResolver"))
	}
	if cm.authInfoResolver == nil {
		errs = append(errs, fmt.Errorf("the clientManager requires an authInfoResolver"))
	}
	return utilerrors.NewAggregate(errs)
}

// HookClient get a RESTClient from the cache, or constructs one based on the
// webhook configuration.
func (cm *ClientManager) HookClient(clientConfig *WebhookClientConfig) (*rest.RESTClient, error) {
	cacheKey, err := json.Marshal(clientConfig)
	if err != nil {
		return nil, err
	}
	if client, ok := cm.cache.Get(string(cacheKey)); ok {
		return client.(*rest.RESTClient), nil
	}

	complete := func(cfg *rest.Config) (*rest.RESTClient, error) {
		// Combine CAData from the config with any existing CA bundle provided
		if len(cfg.TLSClientConfig.CAData) > 0 {
			cfg.TLSClientConfig.CAData = append(cfg.TLSClientConfig.CAData, '\n')
		}
		cfg.TLSClientConfig.CAData = append(cfg.TLSClientConfig.CAData, clientConfig.CABundle...)

		cfg.ContentConfig.NegotiatedSerializer = cm.negotiatedSerializer
		cfg.ContentConfig.ContentType = runtime.ContentTypeJSON
		client, err := rest.UnversionedRESTClientFor(cfg)
		if err == nil {
			cm.cache.Add(string(cacheKey), client)
		}
		return client, err
	}

	if svc := clientConfig.Service; svc != nil {
		restConfig, err := cm.authInfoResolver.ClientConfigForService(svc.Name, svc.Namespace)
		if err != nil {
			return nil, err
		}
		cfg := rest.CopyConfig(restConfig)
		serverName := svc.Name + "." + svc.Namespace + ".svc"
		host := serverName + ":443"
		cfg.Host = "https://" + host
		if svc.Path != nil {
			cfg.APIPath = *svc.Path
		}
		// Set the server name if not already set
		if len(cfg.TLSClientConfig.ServerName) == 0 {
			cfg.TLSClientConfig.ServerName = serverName
		}

		delegateDialer := cfg.Dial
		if delegateDialer == nil {
			var d net.Dialer
			delegateDialer = d.DialContext
		}
		cfg.Dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == host {
				u, err := cm.serviceResolver.ResolveEndpoint(svc.Namespace, svc.Name)
				if err != nil {
					return nil, err
				}
				addr = u.Host
			}
			return delegateDialer(ctx, network, addr)
		}

		return complete(cfg)
	}

	if clientConfig.URL == nil {
		return nil, &webhookerrors.ErrCallingWebhook{Reason: ErrNeedServiceOrURL}
	}

	u, err := url.Parse(*clientConfig.URL)
	if err != nil {
		return nil, &webhookerrors.ErrCallingWebhook{Reason: fmt.Errorf("Unparsable URL: %v", err)}
	}

	restConfig, err := cm.authInfoResolver.ClientConfigFor(u.Host)
	if err != nil {
		return nil, err
	}

	cfg := rest.CopyConfig(restConfig)
	cfg.Host = u.Scheme + "://" + u.Host
	cfg.APIPath = u.Path

	return complete(cfg)
}
