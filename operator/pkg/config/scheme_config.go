package config

import (
	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	spicedbv1alpha1 "github.com/authzed/spicedb-operator/pkg/apis/authzed/v1alpha1"
	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	configv1 "github.com/openshift/api/config/v1"
	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	workv1 "open-cluster-management.io/api/work/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	globalhubv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha1"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
)

func GetRuntimeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta2.AddToScheme(scheme))
	utilruntime.Must(workv1.AddToScheme(scheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(scheme))
	utilruntime.Must(globalhubv1alpha4.AddToScheme(scheme))
	utilruntime.Must(globalhubv1alpha1.AddToScheme(scheme))
	utilruntime.Must(subv1alpha1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(admissionregistrationv1.AddToScheme(scheme))
	utilruntime.Must(mchv1.AddToScheme(scheme))
	utilruntime.Must(agentv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(promv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(postgresv1beta1.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))
	utilruntime.Must(spicedbv1alpha1.AddToScheme(scheme))
	utilruntime.Must(migrationv1alpha1.AddToScheme(scheme))

	// add Kafka scheme
	utilruntime.Must(kafkav1beta2.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	return scheme
}
