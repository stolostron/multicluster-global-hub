package postgres

import (
	"testing"

	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
)

func TestNewSubscription(t *testing.T) {
	sub := NewSubscription(&globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "globalhub",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}, &subv1alpha1.SubscriptionConfig{}, true)

	if sub.Spec.Package != communityPackageName {
		t.Errorf("Expected package name %s, got %s", communityPackageName, sub.Spec.Package)
	}

	sub = NewSubscription(&globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "globalhub",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}, &subv1alpha1.SubscriptionConfig{}, false)

	if sub.Spec.Package != packageName {
		t.Errorf("Expected package name %s, got %s", packageName, sub.Spec.Package)
	}

	sub = NewSubscription(&globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "globalhub",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}, &subv1alpha1.SubscriptionConfig{
		NodeSelector: map[string]string{
			"foo": "bar",
		},
	}, true)

	if sub.Spec.Config.NodeSelector["foo"] != "bar" {
		t.Errorf("Expected nodeSelector name foo value is bar, got %s", sub.Spec.Config.NodeSelector["foo"])
	}
}

func TestRenderSubscription(t *testing.T) {
	sub := RenderSubscription(&subv1alpha1.Subscription{
		Spec: &subv1alpha1.SubscriptionSpec{
			Package: packageName,
			Channel: "foo",
		},
	}, &subv1alpha1.SubscriptionConfig{}, true)
	if sub.Spec.Package != communityPackageName {
		t.Errorf("Expected package name %s, got %s", communityPackageName, sub.Spec.Package)
	}

	sub = RenderSubscription(&subv1alpha1.Subscription{
		Spec: &subv1alpha1.SubscriptionSpec{
			Package: packageName,
			Channel: "foo",
		},
	}, &subv1alpha1.SubscriptionConfig{}, true)
	if sub.Spec.Package != communityPackageName {
		t.Errorf("Expected package name %s, got %s", communityPackageName, sub.Spec.Package)
	}
}

func TestNewPostgres(t *testing.T) {
	kafka := NewPostgres(PostgresName, "default")
	if kafka.Name != PostgresName {
		t.Errorf("Expected name %s, got %s", PostgresName, kafka.Name)
	}
}
