// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package authentication

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	userv1 "github.com/openshift/api/user/v1"
)

const (
	// UserKey - the key for user string in context.
	UserKey = "user"
	// GroupsKey - the key for groups slice of strings in context.
	GroupsKey = "groups"
)

var errUnableToAppendCABundle = errors.New("unable to append CA Bundle")

// Authentication middleware.
func Authentication(clusterAPIURL string, clusterAPICABundle []byte) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		authorizationHeader := ginCtx.GetHeader("Authorization")
		if !setAuthenticatedUser(ginCtx, authorizationHeader, clusterAPIURL, clusterAPICABundle) {
			ginCtx.Header("WWW-Authenticate", "")
			ginCtx.AbortWithStatus(http.StatusUnauthorized)

			return
		}

		ginCtx.Next()
	}
}

func createClient(clusterAPICABundle []byte) (*http.Client, error) {
	tlsConfig := &tls.Config{
		//nolint:gosec
		InsecureSkipVerify: true,
	}

	if clusterAPICABundle != nil {
		rootCAs := x509.NewCertPool()
		if ok := rootCAs.AppendCertsFromPEM(clusterAPICABundle); !ok {
			fmt.Fprintf(gin.DefaultWriter, "unable to append cluster API CA Bundle")
			return nil, fmt.Errorf("unable to append cluster API CA Bundle %w", errUnableToAppendCABundle)
		}

		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    rootCAs,
		}
	}

	tr := &http.Transport{TLSClientConfig: tlsConfig}

	return &http.Client{Transport: tr}, nil
}

func setAuthenticatedUser(ginCtx *gin.Context, authorizationHeader string, clusterAPIURL string,
	clusterAPICABundle []byte,
) bool {
	client, err := createClient(clusterAPICABundle)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "unable to create client: %v\n", err)
	}

	req, err := http.NewRequestWithContext(context.TODO(), "GET", fmt.Sprintf("%s/apis/user.openshift.io/v1/users/~",
		clusterAPIURL), nil)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "unable to create request: %v\n", err)
	}

	req.Header.Add("Authorization", authorizationHeader)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "got authentication error: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "unable to read authentication response body: %v\n", err)
		return false
	}

	user := userv1.User{}

	err = json.Unmarshal(body, &user)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "failed to unmarshall json: %v\n", err)
		return false
	}

	ginCtx.Set(UserKey, user.Name)
	ginCtx.Set(GroupsKey, user.Groups)

	fmt.Fprintf(gin.DefaultWriter, "got authenticated user: %v\n", user.Name)
	fmt.Fprintf(gin.DefaultWriter, "user groups: %v\n", user.Groups)

	return true
}
