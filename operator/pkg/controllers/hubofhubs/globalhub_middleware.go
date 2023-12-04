/*
Copyright 2023.

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

package hubofhubs

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/postgres"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
)

type middlewareResult struct {
	Err           error
	StorageConn   *postgres.PostgresConnection
	TransportConn *transport.ConnCredential
}

// ReconcileMiddleware creates the kafka and postgres if needed.
// 1. create the kafka and postgres subscription at the same time
// 2. then create the kafka and postgres resources at the same time
// 3. wait for kafka and postgres ready
func (r *MulticlusterGlobalHubReconciler) ReconcileMiddleware(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
) (ctrl.Result, error) {
	// initialize postgres and kafka at the same time
	var wg sync.WaitGroup

	resultChan := make(chan middlewareResult, 2)

	// initialize transport
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, e := r.ReconcileTransport(ctx, mgh)
		resultChan <- middlewareResult{
			Err:           e,
			TransportConn: conn,
		}
	}()

	// initialize storage
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, e := r.ReconcileStorage(ctx, mgh)
		resultChan <- middlewareResult{
			Err:         e,
			StorageConn: conn,
		}
	}()

	wg.Done()

	for result := range resultChan {
		if result.Err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("middleware not ready, Error: %v", result.Err)
		}
		if result.StorageConn != nil {
			r.MiddlewareConfig.StorageConn = result.StorageConn
		}
		if result.TransportConn != nil {
			r.MiddlewareConfig.TransportConn = result.TransportConn
		}
	}

	return ctrl.Result{}, nil
}

func (r *MulticlusterGlobalHubReconciler) ReconcileTransport(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
) (*transport.ConnCredential, error) {
	transportProtocol, err := detectTransportProtocol(ctx, r.Client)
	if err != nil {
		return nil, err
	}

	// create the transport instance
	var trans transport.Transporter
	switch transportProtocol {
	case transport.InternalTransport:
		trans, err = protocol.NewStrimziKafka(protocol.WithContext(ctx), protocol.WithClient(r.Client),
			protocol.WithCommunity(utils.IsCommunityMode()), protocol.WithGlobalHub(mgh))
		if err != nil {
			return nil, err
		}
	case transport.ExternalTransport:
		trans = protocol.NewTransportSecret(ctx, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      constants.GHTransportSecretName,
		}, r.Client)
	}

	// create the user to connect the transport instance
	err = trans.CreateUser(protocol.DefaultGlobalHubKafkaUser)
	if err != nil {
		return nil, err
	}
	// create global hub topics

	topics := transport.GetTopicNames(transport.GlobalHubTopicIdentity)
	err = trans.CreateTopic([]string{
		topics.SpecTopic,
		topics.StatusTopic,
		topics.EventTopic,
	})
	if err != nil {
		return nil, err
	}

	var conn *transport.ConnCredential
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			conn, err = trans.GetConnCredential(protocol.DefaultGlobalHubKafkaUser)
			if err != nil {
				r.Log.Info("waiting the kafka connection credential to be ready...", "message", err.Error())
				return false, err
			}
			return true, nil
		})
	if err != nil && trans != nil {
		config.SetTransporter(trans)
	}
	return conn, err
}

func (r *MulticlusterGlobalHubReconciler) ReconcileStorage(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
) (*postgres.PostgresConnection, error) {
	// support BYO postgres
	pgConnection, err := r.GeneratePGConnectionFromGHStorageSecret(ctx)
	if err == nil {
		return pgConnection, nil
	} else if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	// then the storage secret is not found
	// if not-provided postgres secret, create crunchy postgres operator by subscription
	if config.GetInstallCrunchyOperator(mgh) {
		if err := r.EnsureCrunchyPostgresSubscription(ctx, mgh); err != nil {
			return nil, err
		}
	} else {
		// create the statefulset postgres and initialize the r.MiddlewareConfig.PgConnection
		pgConnection, err = r.InitPostgresByStatefulset(ctx, mgh)
		if err != nil {
			return nil, err
		}
	}

	if pgConnection == nil && config.GetInstallCrunchyOperator(mgh) {
		if err := r.EnsureCrunchyPostgres(ctx); err != nil {
			return nil, err
		}
	}

	if pgConnection == nil && config.GetInstallCrunchyOperator(mgh) {
		// store crunchy postgres connection
		err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if err := r.EnsureCrunchyPostgres(ctx); err != nil {
					r.Log.Info("waiting the postgres cluster to be ready...", "message", err.Error())
					return false, nil
				}

				pgConnection, err = r.WaitForPostgresReady(ctx)
				if err != nil {
					r.Log.Info("waiting the postgres connection credential to be ready...", "message", err.Error())
					return false, nil
				}
				return true, nil
			})
	}
	return pgConnection, nil
}

func detectTransportProtocol(ctx context.Context, runtimeClient client.Client) (transport.TransportProtocol, error) {
	// get the transport secret
	kafkaSecret := &corev1.Secret{}
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaSecret)
	if err == nil {
		return transport.ExternalTransport, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return transport.ExternalTransport, err
	}

	// the transport secret is not found
	return transport.InternalTransport, nil
}
