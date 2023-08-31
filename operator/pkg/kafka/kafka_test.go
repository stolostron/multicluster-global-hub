package kafka

import (
	"testing"

	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestNewKafka(t *testing.T) {
	kafka := NewKafka("foo", "bar")
	if kafka.Name != "foo" {
		t.Errorf("Expected name foo, got %s", kafka.Name)
	}
	if kafka.Namespace != "bar" {
		t.Errorf("Expected namespace bar, got %s", kafka.Namespace)
	}
}

func TestNewKafkaTopic(t *testing.T) {
	kafkaTopic := NewKafkaTopic("foo", "bar")
	if kafkaTopic.Name != "foo" {
		t.Errorf("Expected name foo, got %s", kafkaTopic.Name)
	}
	if kafkaTopic.Namespace != "bar" {
		t.Errorf("Expected namespace bar, got %s", kafkaTopic.Namespace)
	}
}

func TestNewKafkaUser(t *testing.T) {
	kafkaUser := NewKafkaUser("foo", "bar")
	if kafkaUser.Name != "foo" {
		t.Errorf("Expected name foo, got %s", kafkaUser.Name)
	}
	if kafkaUser.Namespace != "bar" {
		t.Errorf("Expected namespace bar, got %s", kafkaUser.Namespace)
	}
}
