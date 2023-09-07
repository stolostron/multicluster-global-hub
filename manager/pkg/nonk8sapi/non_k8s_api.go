// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package nonk8sapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/authentication"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/managedclusters"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/policies"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/subscriptions"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
)

const secondsToFinishOnShutdown = 5

var errFailedToLoadCertificate = errors.New("failed to load certificate/key")

type NonK8sAPIServerConfig struct {
	ClusterAPIURL          string
	ClusterAPICABundlePath string
	ServerBasePath         string
}

// NeedLeaderElection implements the LeaderElectionRunnable interface, which indicates
// the nonK8sApiServer doesn't need leader election.
func (*nonK8sApiServer) NeedLeaderElection() bool {
	return false
}

// nonK8sApiServer defines the non-k8s-api-server
type nonK8sApiServer struct {
	log logr.Logger
	svr *http.Server
}

func readCertificateAuthority(nonK8sAPIServerConfig *NonK8sAPIServerConfig) ([]byte, error) {
	var clusterAPICABundle []byte

	if nonK8sAPIServerConfig.ClusterAPICABundlePath != "" {
		clusterAPICABundle, err := os.ReadFile(nonK8sAPIServerConfig.ClusterAPICABundlePath)
		if err != nil {
			return clusterAPICABundle,
				fmt.Errorf("%w: %s", errFailedToLoadCertificate,
					nonK8sAPIServerConfig.ClusterAPICABundlePath)
		}
	}

	return clusterAPICABundle, nil
}

// AddNonK8sApiServer adds the non-k8s-api-server to the Manager.
func AddNonK8sApiServer(mgr ctrl.Manager, database db.DB, nonK8sAPIServerConfig *NonK8sAPIServerConfig) error {
	router, err := SetupRouter(database, nonK8sAPIServerConfig)
	if err != nil {
		return err
	}

	err = mgr.Add(&nonK8sApiServer{
		log: ctrl.Log.WithName("non-k8s-api-server"),
		svr: &http.Server{
			Addr:              ":8080",
			Handler:           router,
			ReadHeaderTimeout: time.Minute * 1,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add non k8s api server to the manager: %w", err)
	}

	return nil
}

// @title         Multicluster Global Hub API
// @version       1.0.0
// @description   This documentation is for the APIs of multicluster global hub resources for {product-title}.

// @contact.name  acm-contact
// @contact.email acm-contact@redhat.com
// @contact.url   https://github.com/stolostron/multicluster-global-hub

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @basePath      /global-hub-api/v1
// @schemes       http https
// @securityDefinitions.apikey  ApiKeyAuth
// @in                          header
// @name                        Authorization
// @description					Authorization with user access token
func SetupRouter(database db.DB, nonK8sAPIServerConfig *NonK8sAPIServerConfig) (*gin.Engine, error) {
	router := gin.Default()
	// add aythentication eith openshift oauth
	// skip authentication middleware if ClusterAPIURL is empty for testing
	if nonK8sAPIServerConfig.ClusterAPIURL != "" {
		clusterAPICABundle, err := readCertificateAuthority(nonK8sAPIServerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificates authority: %w", err)
		}
		router.Use(authentication.Authentication(nonK8sAPIServerConfig.ClusterAPIURL, clusterAPICABundle))
	}

	routerGroup := router.Group(nonK8sAPIServerConfig.ServerBasePath)
	routerGroup.GET("/managedclusters", managedclusters.ListManagedClusters(database.GetConn()))
	routerGroup.PATCH("/managedcluster/:clusterID",
		managedclusters.PatchManagedCluster(database.GetConn()))
	routerGroup.GET("/policies", policies.ListPolicies(database.GetConn()))
	routerGroup.GET("/policy/:policyID/status", policies.GetPolicyStatus(database.GetConn()))
	routerGroup.GET("/subscriptions", subscriptions.ListSubscriptions(database.GetConn()))
	routerGroup.GET("/subscriptionreport/:subscriptionID",
		subscriptions.GetSubscriptionReport(database.GetConn()))

	return router, nil
}

// Start runs the non-k8s-api server within given context
func (s *nonK8sApiServer) Start(ctx context.Context) error {
	idleConnsClosed := make(chan struct{})
	// initializing the shutdown process in a goroutine so that it won't block the server starting and running
	go func() {
		<-ctx.Done()
		s.log.Info("shutting down non-k8s-api server")

		// The context is used to inform the server it has 5 seconds to finish the request it is currently handling
		shutdownCtx, cancel := context.WithTimeout(context.Background(), secondsToFinishOnShutdown*time.Second)
		defer cancel()
		if err := s.svr.Shutdown(shutdownCtx); err != nil {
			// Error from closing listeners, or context timeout
			s.log.Error(err, "error shutting down the non-k8s-api server")
		}

		s.log.Info("the non-k8s-api server is exiting")
		close(idleConnsClosed)
	}()

	if err := s.svr.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-idleConnsClosed
	return nil
}
