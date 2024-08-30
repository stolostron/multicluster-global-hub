// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	name      = "global-hub"
	namespace = utils.GetDefaultNamespace()
)

func getMGH() *v1alpha4.MulticlusterGlobalHub {
	return &v1alpha4.MulticlusterGlobalHub{
		TypeMeta:   metav1.TypeMeta{Kind: "MulticlusterGlobalHub"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       v1alpha4.MulticlusterGlobalHubSpec{},
	}
}

func getExpiredCertSecret() *v1.Secret {
	date := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country: []string{"US"},
		},
		NotBefore: date,
		NotAfter:  date.AddDate(1, 0, 0),
		IsCA:      true,
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
	}
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	caBytes, _ := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	certPEM, keyPEM := pemEncode(caBytes, x509.MarshalPKCS1PrivateKey(caKey))
	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCACerts,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt":  caBytes,
			"tls.crt": append(certPEM.Bytes(), certPEM.Bytes()...),
			"tls.key": keyPEM.Bytes(),
		},
	}
	return caSecret
}

func TestCreateCertificates(t *testing.T) {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inventory-api",
			Namespace: namespace,
		},
		Spec: routev1.RouteSpec{
			Host: "apiServerURL",
		},
	}
	mgh := getMGH()
	s := scheme.Scheme
	v1alpha4.SchemeBuilder.AddToScheme(s)
	routev1.AddToScheme(s)

	c := fake.NewClientBuilder().WithRuntimeObjects(route).Build()

	err := CreateInventoryCerts(context.TODO(), c, s, mgh)
	if err != nil {
		t.Fatalf("CreateObservabilityCerts: (%v)", err)
	}

	err = CreateInventoryCerts(context.TODO(), c, s, mgh)
	if err != nil {
		t.Fatalf("Rerun CreateObservabilityCerts: (%v)", err)
	}

	err, _ = createCASecret(c, s, mgh, true, serverCACerts, mgh.Namespace, serverCACertificateCN)
	if err != nil {
		t.Fatalf("Failed to renew server ca certificates: (%v)", err)
	}

	err = createCertSecret(c, s, mgh, true, guestCerts, mgh.Namespace, false, guestCertificateCN, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to renew server certificates: (%v)", err)
	}
}

func TestRemoveExpiredCA(t *testing.T) {
	caSecret := getExpiredCertSecret()
	oldCertLength := len(caSecret.Data["tls.crt"])
	c := fake.NewClientBuilder().WithRuntimeObjects(caSecret).Build()
	removeExpiredCA(c, serverCACerts, namespace)
	c.Get(context.TODO(),
		types.NamespacedName{Name: serverCACerts, Namespace: namespace},
		caSecret)
	if len(caSecret.Data["tls.crt"]) != oldCertLength/2 {
		t.Fatal("Expired certificate not removed correctly")
	}
}
