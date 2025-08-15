// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SendWatchEvent(watchEvent *metav1.WatchEvent, writer io.Writer) error {
	json, err := json.Marshal(watchEvent)
	if err != nil {
		return err
	}

	_, err = writer.Write(json)
	if err != nil {
		return err
	}

	_, err = writer.Write([]byte("\n"))

	return err
}

func ShouldReturnAsTable(ginCtx *gin.Context) bool {
	acceptFormat, containAcceptFormat := "application/json", false
	acceptAs, containeAcceptAs := "as=Table", false
	acceptGroup, containeAcceptGroup := fmt.Sprintf("g=%s", metav1.GroupName), false
	acceptVersion, containAcceptVersion := fmt.Sprintf("v=%s",
		metav1.SchemeGroupVersion.Version), false

	// implement the real negotiation logic here (with weights)
	// see https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html
	for _, accepted := range strings.Split(ginCtx.GetHeader("Accept"), ",") {
		acceptedList := strings.Split(accepted, ";")
		containAcceptFormat = false
		containeAcceptAs = false
		containeAcceptGroup = false
		containAcceptVersion = false
		for _, v := range acceptedList {
			switch v {
			case acceptFormat:
				containAcceptFormat = true
			case acceptAs:
				containeAcceptAs = true
			case acceptGroup:
				containeAcceptGroup = true
			case acceptVersion:
				containAcceptVersion = true
			}
		}
		if containAcceptFormat && containeAcceptAs && containeAcceptGroup && containAcceptVersion {
			return true
		}
	}

	return false
}
