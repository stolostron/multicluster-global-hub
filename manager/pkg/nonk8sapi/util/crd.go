// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// GetCustomResourceColumnDefinitions return the column definitions for CRD `name`, version `version`.
func GetCustomResourceColumnDefinitions(name, version string) []apiextensionsv1.CustomResourceColumnDefinition {
	// nolint:lll
	// from https://github.com/kubernetes/apiextensions-apiserver/blob/76c7ff37eea5429706c9cfb4a0e9215e49d93930/pkg/apis/apiextensions/helpers.go#L243
	defaultColumns := []apiextensionsv1.CustomResourceColumnDefinition{
		{Name: "Age", Type: "date", JSONPath: ".metadata.creationTimestamp"},
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "unable to get inCluster config %s\n", err.Error())
		return defaultColumns
	}

	theClientSet, err := clientset.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "unable to create new client set %s\n", err.Error())
		return defaultColumns
	}

	// nolint:lll
	// from https://github.com/kubernetes/apiextensions-apiserver/blob/76c7ff37eea5429706c9cfb4a0e9215e49d93930/test/integration/helpers.go#L84

	crdv1, err := theClientSet.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "failed to get CustomResourceDefinition for %s: %v", name, err)
		return defaultColumns
	}

	var crd apiextensions.CustomResourceDefinition
	err = apiextensionsv1.Convert_v1_CustomResourceDefinition_To_apiextensions_CustomResourceDefinition(crdv1, &crd, nil)

	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "failed to convert crd v1 to crd for %s: %v", name, err)
		return defaultColumns
	}

	columns, err := apiextensions.GetColumnsForVersion(&crd, version)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "failed to get columns for version for %s CRD: %s %v", name, version, err)
		return defaultColumns
	}

	if len(columns) < 1 {
		return defaultColumns
	}

	columnsv1, err := convertColumnsToColumnsV1(columns)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter,
			"failed to convert columns (from apiextensions to apiextensions/v1) for %s CRD: %s %v", name, version, err)
		return defaultColumns
	}

	return columnsv1
}

func convertColumnsToColumnsV1(columns []apiextensions.CustomResourceColumnDefinition) (
	[]apiextensionsv1.CustomResourceColumnDefinition, error,
) {
	columnsv1 := make([]apiextensionsv1.CustomResourceColumnDefinition, 0, len(columns))

	for _, column := range columns {
		var columnv1 apiextensionsv1.CustomResourceColumnDefinition

		columnCopy := column.DeepCopy()

		// nolint:lll
		err := apiextensionsv1.Convert_apiextensions_CustomResourceColumnDefinition_To_v1_CustomResourceColumnDefinition(columnCopy,
			&columnv1, nil)
		if err != nil {
			return nil, fmt.Errorf("unable to convert column: %w", err)
		}

		columnsv1 = append(columnsv1, *columnv1.DeepCopy())
	}

	return columnsv1, nil
}
