// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
)

func init() {
	s := scheme.Scheme
	v1alpha4.SchemeBuilder.AddToScheme(s)
}

func TestOnDelete(t *testing.T) {
	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InventoryServerCASecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("new cert-"),
		},
	}
	deletCaSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InventoryServerCASecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("old cert"),
		},
	}
	c := fake.NewClientBuilder().WithRuntimeObjects(caSecret, getMGH()).Build()
	onDelete(c)(deletCaSecret)
	c.Get(context.TODO(), types.NamespacedName{Name: InventoryServerCASecretName, Namespace: namespace}, caSecret)
	data := string(caSecret.Data["tls.crt"])
	if data != "new cert-" {
		t.Fatalf("deleted cert not added back: %s", data)
	}
}

func TestOnUpdate(t *testing.T) {
	certSecret := getExpiredCertSecret()
	oldCertLength := len(certSecret.Data["tls.crt"])
	c := fake.NewClientBuilder().WithRuntimeObjects(certSecret).Build()
	onUpdate(context.TODO(), c)(certSecret, certSecret)
	certSecret.Name = InventoryClientCASecretName
	onUpdate(context.TODO(), c)(certSecret, certSecret)
	certSecret.Name = guestCerts
	onUpdate(context.TODO(), c)(certSecret, certSecret)
	certSecret.Name = serverCerts
	onUpdate(context.TODO(), c)(certSecret, certSecret)
	c.Get(context.TODO(), types.NamespacedName{Name: InventoryServerCASecretName, Namespace: namespace}, certSecret)
	if len(certSecret.Data["tls.crt"]) <= oldCertLength {
		t.Fatal("certificate not renewed correctly")
	}
}
