// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package nonk8sapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/authentication"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/managedclusters"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
)

const secondsToFinishOnShutdown = 5

var errFailedToLoadCertificate = errors.New("failed to load certificate/key")

type NonK8sAPIServerConfig struct {
	ClusterAPIURL             string
	ClusterAPICABundlePath    string
	AuthorizationURL          string
	AuthorizationCABundlePath string
	ServerCertificatePath     string
	ServerKeyPath             string
	ServerBasePath            string
}

// nonK8sApiServer defines the non-k8s-api-server
type nonK8sApiServer struct {
	log             logr.Logger
	certificateFile string
	keyFile         string
	svr             *http.Server
}

func readCertificates(nonK8sAPIServerConfig *NonK8sAPIServerConfig) ([]byte, []byte, tls.Certificate, error) {
	var (
		clusterAPICABundle    []byte
		authorizationCABundle []byte
		certificate           tls.Certificate
	)

	if nonK8sAPIServerConfig.ClusterAPICABundlePath != "" {
		clusterAPICABundle, err := ioutil.ReadFile(nonK8sAPIServerConfig.ClusterAPICABundlePath)
		if err != nil {
			return clusterAPICABundle, authorizationCABundle, certificate,
				fmt.Errorf("%w: %s", errFailedToLoadCertificate,
					nonK8sAPIServerConfig.ClusterAPICABundlePath)
		}
	}

	if nonK8sAPIServerConfig.AuthorizationCABundlePath != "" {
		authorizationCABundle, err := ioutil.ReadFile(
			nonK8sAPIServerConfig.AuthorizationCABundlePath)
		if err != nil {
			return clusterAPICABundle, authorizationCABundle, certificate,
				fmt.Errorf("%w: %s", errFailedToLoadCertificate,
					nonK8sAPIServerConfig.AuthorizationCABundlePath)
		}
	}

	certificate, err := tls.LoadX509KeyPair(nonK8sAPIServerConfig.ServerCertificatePath, nonK8sAPIServerConfig.ServerKeyPath)
	if err != nil {
		return clusterAPICABundle, authorizationCABundle, certificate,
			fmt.Errorf("%w: %s/%s", errFailedToLoadCertificate,
				nonK8sAPIServerConfig.ServerCertificatePath, nonK8sAPIServerConfig.ServerKeyPath)
	}

	return clusterAPICABundle, authorizationCABundle, certificate, nil
}

// AddNonK8sApiServer adds the non-k8s-api-server to the Manager.
func AddNonK8sApiServer(mgr ctrl.Manager, database db.DB, nonK8sAPIServerConfig *NonK8sAPIServerConfig) error {
	// read the certificate of non-k8s-api server
	clusterAPICABundle, authorizationCABundle, _, err := readCertificates(nonK8sAPIServerConfig)
	if err != nil {
		return fmt.Errorf("failed to read certificates: %w", err)
	}

	router := gin.Default()
	router.Use(authentication.Authentication(nonK8sAPIServerConfig.ClusterAPIURL, clusterAPICABundle))

	routerGroup := router.Group(nonK8sAPIServerConfig.ServerBasePath)
	routerGroup.GET("/managedclusters", managedclusters.List(
		nonK8sAPIServerConfig.AuthorizationURL, authorizationCABundle, database.GetConn()))
	routerGroup.PATCH("/managedclusters/:cluster",
		managedclusters.Patch(nonK8sAPIServerConfig.AuthorizationURL, authorizationCABundle, database.GetConn()))

	err = mgr.Add(&nonK8sApiServer{
		log: ctrl.Log.WithName("non-k8s-api-server"),
		svr: &http.Server{
			Addr:              ":8080",
			Handler:           router,
			ReadHeaderTimeout: time.Minute * 1,
		},
		certificateFile: nonK8sAPIServerConfig.ServerCertificatePath,
		keyFile:         nonK8sAPIServerConfig.ServerKeyPath,
	})
	if err != nil {
		return fmt.Errorf("failed to add non k8s api server to the manager: %w", err)
	}

	return nil
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

	if err := s.svr.ListenAndServeTLS(s.certificateFile, s.keyFile); err != nil && errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-idleConnsClosed
	return nil
}
