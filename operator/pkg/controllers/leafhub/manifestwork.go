/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leafhub

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-logr/logr"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/stolostron/multicluster-globalhub/operator/apis/operator/v1alpha1"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/config"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/constants"
)

//go:embed manifests/nonhypershift
var nonHypershiftManifestFS embed.FS

//go:embed manifests/hypershift/hub
var hypershiftHubManifestFS embed.FS

//go:embed manifests/hypershift/agent
var hypershiftAgentManifestFS embed.FS

type ACMImageEntry struct {
	CertPolicyController              string
	IAMPolicyController               string
	ConfigPolicyController            string
	GovernancePolicyStatusSync        string
	GovernancePolicySpecSync          string
	GovernancePolicyTemplateSync      string
	GovernancePolicyAddonController   string
	GovernancePolicyPropagator        string
	MulticlusterOperatorsChannel      string
	MulticlusterOperatorsSubscription string
}

type MCEImageEntry struct {
	DefaultImageRegistry           string
	Registration                   string
	RegistrationOperator           string
	ManagedClusterImportController string
	Work                           string
	Placement                      string
}

type HypershiftHubConfigValues struct {
	HubVersion        string
	HoHAgentImage     string
	HostedClusterName string
	ImagePullSecret   string
	ChannelClusterIP  string
	ACM               ACMImageEntry
	MCE               MCEImageEntry
}

type HoHAgentConfigValues struct {
	HoHAgentImage          string
	LeadHubID              string
	KafkaBootstrapServer   string
	KafkaCA                string
	HostedClusterNamespace string // for hypershift case
}

var podNamespace, imagePullSecretName string

func init() {
	podNamespace, _ = os.LookupEnv("POD_NAMESPACE")
	if podNamespace == "" {
		podNamespace = constants.HOHDefaultNamespace
	}

	imagePullSecretName = "multiclusterhub-operator-pull-secret"
}

// applyHubSubWork creates or updates the subscription manifestwork for leafhub cluster
func applyHubSubWork(ctx context.Context, c client.Client, log logr.Logger, managedClusterName string,
	pm *packageManifestConfig,
) (*workv1.ManifestWork, error) {
	desiredHubSubWork, err := buildHubSubWork(ctx, c, log, managedClusterName, pm)
	if err != nil {
		return nil, err
	}

	existingHubSubWork := &workv1.ManifestWork{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name: fmt.Sprintf("%s-%s", managedClusterName,
				constants.HOHHubSubscriptionWorkSuffix),
			Namespace: managedClusterName,
		}, existingHubSubWork); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating hub subscription manifestwork",
				"namespace", desiredHubSubWork.GetNamespace(), "name", desiredHubSubWork.GetName())
			return desiredHubSubWork, c.Create(ctx, desiredHubSubWork)
		} else {
			// Error reading the object
			return nil, err
		}
	}

	// Do not need to update if the packagemanifest is changed
	// for example: the existing DefaultChannel is release-2.4, once the new release is delivered.
	// the DefaultChannel will be release-2.5. but we do not need to update the manifestwork.
	existingPM, err := getPackageManifestConfigFromHubSubWork(existingHubSubWork)
	if err != nil {
		return nil, err
	}
	log.Info("existing packagemanifest", "packagemanifest", existingPM, "managedcluster", managedClusterName)
	desiredHubSubWork, err = buildHubSubWork(ctx, c, log, managedClusterName, existingPM)
	if err != nil {
		return nil, err
	}

	modified, err := ensureManifestWork(desiredHubSubWork, existingHubSubWork, log)
	if err != nil {
		return nil, err
	}
	if modified {
		log.Info("updating hub subscription manifestwork",
			"namespace", desiredHubSubWork.GetNamespace(), "name", desiredHubSubWork.GetName())
		desiredHubSubWork.ObjectMeta.ResourceVersion =
			existingHubSubWork.ObjectMeta.ResourceVersion
		return desiredHubSubWork, c.Update(ctx, desiredHubSubWork)
	}

	log.Info("hub subscription manifestwork doesn't change",
		"namespace", desiredHubSubWork.GetNamespace(), "name", desiredHubSubWork.GetName())
	return existingHubSubWork, nil
}

// buildHubSubWork creates hub subscription manifestwork
func buildHubSubWork(ctx context.Context, c client.Client, log logr.Logger, managedClusterName string,
	pm *packageManifestConfig,
) (*workv1.ManifestWork, error) {
	tpl, err := parseNonHypershiftTemplates(nonHypershiftManifestFS)
	if err != nil {
		return nil, err
	}

	hubSubConfigValues := struct {
		Channel               string
		CurrentCSV            string
		Source                string
		SourceNamespace       string
		InstallationNamespace string
	}{
		Channel:               pm.ACMDefaultChannel,
		CurrentCSV:            pm.ACMCurrentCSV,
		Source:                constants.ACMSubscriptionPublicSource,
		SourceNamespace:       constants.OpenshiftMarketPlaceNamespace,
		InstallationNamespace: constants.HOHDefaultNamespace,
	}

	var buf bytes.Buffer
	if err = tpl.ExecuteTemplate(&buf, "manifests/nonhypershift/subscription", hubSubConfigValues); err != nil {
		return nil, err
	}
	// log.Info("templates for subscription objects", buf.String())

	subManifests := []workv1.Manifest{}
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(&buf))
	for {
		b, err := yamlReader.Read()
		if err == io.EOF {
			buf.Reset()
			break
		}
		if err != nil {
			return nil, err
		}
		if len(b) != 0 {
			rawJSON, err := yaml.ToJSON(b)
			if err != nil {
				return nil, err
			}
			if string(rawJSON) != "null" {
				// log.Info("raw JSON object for subscription", "json", rawJSON)
				subManifests = append(subManifests, workv1.Manifest{
					RawExtension: runtime.RawExtension{Raw: rawJSON},
				})
			}
		}
	}

	imagePullSecret, err := generatePullSecret(ctx, c, constants.OpenshiftMarketPlaceNamespace)
	if err != nil {
		return nil, err
	}
	if imagePullSecret != nil {
		subManifests = append(subManifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Object: imagePullSecret},
		})
	}

	hubSubWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", managedClusterName,
				constants.HOHHubSubscriptionWorkSuffix),
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
			Annotations: map[string]string{
				// Add the postpone delete annotation for manifestwork so that the observabilityaddon can be
				// cleaned up before the manifestwork is deleted by the managedcluster-import-controller when
				// the corresponding managedcluster is detached.
				// Note the annotation value is currently not taking effect, because managedcluster-import-controller
				// managedcluster-import-controller hard code the value to be 10m
				constants.WorkPostponeDeleteAnnotationKey: "",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: subManifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     "operators.coreos.com",
						Resource:  "subscriptions",
						Name:      "acm-operator-subscription",
						Namespace: constants.HOHDefaultNamespace,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "state",
									Path: ".status.state",
								},
							},
						},
					},
				},
			},
		},
	}

	return hubSubWork, nil
}

// getPackageManifestConfigFromHubSubWork retrieves ACM packagemanifest config from hub subscription manifestwork
func getPackageManifestConfigFromHubSubWork(hubSubWork *workv1.ManifestWork) (*packageManifestConfig, error) {
	for _, manifest := range hubSubWork.Spec.Workload.Manifests {
		if strings.Contains(string(manifest.RawExtension.Raw), `"kind":"Subscription"`) {
			sub := olmv1alpha1.Subscription{}
			err := json.Unmarshal(manifest.RawExtension.Raw, &sub)
			if err != nil {
				return nil, err
			}
			return &packageManifestConfig{
				ACMDefaultChannel: sub.Spec.Channel,
				ACMCurrentCSV:     sub.Spec.StartingCSV,
			}, nil
		}
	}
	return nil, nil
}

// applyHubMCHWork creates or updates the mch manifestwork for leafhub cluster
func applyHubMCHWork(ctx context.Context, c client.Client, log logr.Logger,
	managedClusterName string,
) (*workv1.ManifestWork, error) {
	desiredHubMCHWork, err := buildHubMCHWork(ctx, c, log, managedClusterName)
	if err != nil {
		return nil, err
	}

	// creates or updates the hub mch manifestwork
	return applyManifestWork(ctx, c, log, desiredHubMCHWork)
}

// buildHubMCHWork creates hub MCH manifestwork
func buildHubMCHWork(ctx context.Context, c client.Client, log logr.Logger,
	managedClusterName string,
) (*workv1.ManifestWork, error) {
	tpl, err := parseNonHypershiftTemplates(nonHypershiftManifestFS)
	if err != nil {
		return nil, err
	}

	hubMCHConfigValues := struct {
		InstallationNamespace string
	}{
		InstallationNamespace: constants.HOHDefaultNamespace,
	}

	var buf bytes.Buffer
	if err = tpl.ExecuteTemplate(&buf, "manifests/nonhypershift/mch", hubMCHConfigValues); err != nil {
		return nil, err
	}
	// log.Info("render templates for mch objects", "templates", buf.String())

	mchManifests := []workv1.Manifest{}
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(&buf))
	for {
		b, err := yamlReader.Read()
		if err == io.EOF {
			buf.Reset()
			break
		}
		if err != nil {
			return nil, err
		}
		if len(b) != 0 {
			rawJSON, err := yaml.ToJSON(b)
			if err != nil {
				return nil, err
			}
			if string(rawJSON) != "null" {
				// log.Info("raw JSON object for mch", "json", rawJSON)
				mchManifests = append(mchManifests, workv1.Manifest{
					RawExtension: runtime.RawExtension{Raw: rawJSON},
				})
			}
		}
	}

	imagePullSecret, err := generatePullSecret(ctx, c, constants.HOHDefaultNamespace)
	if err != nil {
		return nil, err
	}
	if imagePullSecret != nil {
		mchManifests = append(mchManifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Object: imagePullSecret},
		})
	}

	hubMCHWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHubMCHWorkSuffix),
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: mchManifests,
			},
			DeleteOption: &workv1.DeleteOption{
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     "operator.open-cluster-management.io",
						Resource:  "multiclusterhubs",
						Name:      "multiclusterhub",
						Namespace: constants.HOHDefaultNamespace,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "state",
									Path: ".status.phase",
								},
							},
						},
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "currentVersion",
									Path: ".status.currentVersion",
								},
							},
						},
						// ideally, the mch status should be in Running state.
						// but due to this bug - https://github.com/stolostron/backlog/issues/20555
						// the mch status can be in Installing for a long time.
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "cluster-manager-cr-status",
									Path: ".status.components.cluster-manager-cr.status",
								},
							},
						},
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "multicluster-engine-status",
									Path: ".status.components.multicluster-engine.status",
								},
							},
						},
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "grc-sub-status",
									Path: ".status.components.grc-sub.status",
								},
							},
						},
					},
				},
			},
		},
	}

	return hubMCHWork, nil
}

func getDefaultHypershiftHubConfigValues() HypershiftHubConfigValues {
	return HypershiftHubConfigValues{
		ACM: ACMImageEntry{
			CertPolicyController:              "cert-policy-controller",
			IAMPolicyController:               "iam-policy-controller",
			ConfigPolicyController:            "config-policy-controller",
			GovernancePolicyStatusSync:        "governance-policy-status-sync",
			GovernancePolicySpecSync:          "governance-policy-spec-sync",
			GovernancePolicyTemplateSync:      "governance-policy-template-sync",
			GovernancePolicyAddonController:   "governance-policy-addon-controller",
			GovernancePolicyPropagator:        "governance-policy-propagator",
			MulticlusterOperatorsChannel:      "multicluster-operators-channel",
			MulticlusterOperatorsSubscription: "multicluster-operators-subscription",
		},
		MCE: MCEImageEntry{
			Registration:                   "registration",
			RegistrationOperator:           "registration-operator",
			ManagedClusterImportController: "managedcluster-import-controller",
			Work:                           "work",
			Placement:                      "placement",
		},
	}
}

// applyHubHypershiftWorks apply hub components manifestwork to hypershift hosting and hosted cluster
func applyHubHypershiftWorks(ctx context.Context, c client.Client, log logr.Logger,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, managedClusterName, channelClusterIP string,
	pm *packageManifestConfig, hcConfig *config.HostedClusterConfig,
) (*workv1.ManifestWork, error) {
	if pm == nil || pm.ACMCurrentCSV == "" {
		return nil, fmt.Errorf("empty packagemanifest")
	}

	hypershiftHubConfigValues := getDefaultHypershiftHubConfigValues()
	acmImages, mceImages := pm.ACMImages, pm.MCEImages
	acmDefaultImageRegistry := constants.DefaultACMUpstreamImageRegistry
	mceDefaultImageRegistry := constants.DefaultMCEUpstreamImageRegistry

	acmSnapshot, ok := mgh.GetAnnotations()[constants.HoHHubACMSnapShotKey]
	if !ok || acmSnapshot == "" {
		acmDefaultImageRegistry = constants.DefaultACMDownStreamImageRegistry
		// handle special case for governance-policy-addon-controller image
		hypershiftHubConfigValues.ACM.GovernancePolicyAddonController =
			"acm-governance-policy-addon-controller"
	}
	mceSnapshot, ok := mgh.GetAnnotations()[constants.HoHHubMCESnapShotKey]
	if !ok || mceSnapshot == "" {
		mceDefaultImageRegistry = constants.DefaultMCEDownStreamImageRegistry
	}

	tpl, err := parseHubHypershiftTemplates(hypershiftHubManifestFS, acmSnapshot, mceSnapshot,
		acmDefaultImageRegistry, mceDefaultImageRegistry, acmImages, mceImages)
	if err != nil {
		return nil, err
	}

	hypershiftHostedClusterName := fmt.Sprintf("%s-%s", hcConfig.HostingNamespace, hcConfig.HostedClusterName)
	latestACMVersion := strings.TrimPrefix(pm.ACMCurrentCSV, "advanced-cluster-management.v")
	latestACMVersionParts := strings.Split(latestACMVersion, ".")
	if len(latestACMVersionParts) < 2 {
		return nil, fmt.Errorf("invalid ACM version :%s", latestACMVersion)
	}
	latestACMVersionM := strings.Join(latestACMVersionParts[:2], ".")
	hypershiftHubConfigValues.HubVersion = latestACMVersionM
	hypershiftHubConfigValues.HoHAgentImage = config.GetImage(mgh.GetAnnotations(), "hub_of_hubs_agent")
	hypershiftHubConfigValues.HostedClusterName = hypershiftHostedClusterName
	hypershiftHubConfigValues.ImagePullSecret = imagePullSecretName
	hypershiftHubConfigValues.MCE.DefaultImageRegistry = mceDefaultImageRegistry

	if channelClusterIP != "" {
		hypershiftHubConfigValues.ChannelClusterIP = channelClusterIP
	} else {
		hypershiftHubConfigValues.ChannelClusterIP = ""
	}

	// apply manifestwork on hypershift hosted cluster
	var buf1 bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf1, "manifests/hypershift/hub/hosted",
		hypershiftHubConfigValues); err != nil {
		return nil, err
	}
	// log.Info("render templates for objects on hosted cluster", "templates", buf1.String())

	hostedHubManifests, err := generateWorkManifestsFromBuffer(&buf1)
	if err != nil {
		return nil, err
	}

	imagePullSecret, err := generatePullSecret(ctx, c, hypershiftHostedClusterName)
	if err != nil {
		return nil, err
	}
	if imagePullSecret != nil {
		hostedHubManifests = append(hostedHubManifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Object: imagePullSecret},
		})
	}

	hostedHubWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostedHubWorkSuffix),
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: hostedHubManifests,
			},
			// DeleteOption: &workv1.DeleteOption{
			// 	PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			// },
		},
	}

	if _, err := applyManifestWork(ctx, c, log, hostedHubWork); err != nil {
		return nil, err
	}

	// manifestwork on hypershift hosting cluster
	var buf2 bytes.Buffer
	if err = tpl.ExecuteTemplate(&buf2, "manifests/hypershift/hub/hosting",
		hypershiftHubConfigValues); err != nil {
		return nil, err
	}
	// log.Info("render templates for objects on hosting cluster", "templates", buf1.String())

	hostingHubManifests, err := generateWorkManifestsFromBuffer(&buf2)
	if err != nil {
		return nil, err
	}

	if imagePullSecret != nil {
		hostingHubManifests = append(hostingHubManifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Object: imagePullSecret},
		})
	}

	hostingHubWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostingHubWorkSuffix),
			Namespace: hcConfig.HostingClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{Manifests: hostingHubManifests},
			// DeleteOption: &workv1.DeleteOption{PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan},
			ManifestConfigs: []workv1.ManifestConfigOption{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     "",
						Resource:  "services",
						Name:      "channels-apps-open-cluster-management-webhook-svc",
						Namespace: hypershiftHostedClusterName,
					},
					FeedbackRules: []workv1.FeedbackRule{
						{
							Type: workv1.JSONPathsType,
							JsonPaths: []workv1.JsonPath{
								{
									Name: "clusterIP",
									Path: ".spec.clusterIP",
								},
							},
						},
					},
				},
			},
		},
	}

	return applyManifestWork(ctx, c, log, hostingHubWork)
}

// generateWorkManifestsFromBuffer builds the resource manifests from given buffer
func generateWorkManifestsFromBuffer(buf *bytes.Buffer) ([]workv1.Manifest, error) {
	workManifests := []workv1.Manifest{}
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(buf))
	for {
		b, err := yamlReader.Read()
		if err == io.EOF {
			buf.Reset()
			break
		}
		if err != nil {
			return workManifests, err
		}
		if len(b) != 0 {
			rawJSON, err := yaml.ToJSON(b)
			if err != nil {
				return workManifests, err
			}
			if string(rawJSON) != "null" {
				// log.Info("raw JSON object for object on hosting cluster", "json", rawJSON)
				workManifests = append(workManifests, workv1.Manifest{
					RawExtension: runtime.RawExtension{Raw: rawJSON},
				})
			}
		}
	}

	return workManifests, nil
}

// applyHoHAgentWork creates or updates multicluster-globalhub-agent manifestwork
func applyHoHAgentWork(ctx context.Context, c client.Client, log logr.Logger, mgh *operatorv1alpha1.MultiClusterGlobalHub,
	managedClusterName string,
) error {
	kafkaBootstrapServer, kafkaCA, err := getKafkaConfig(ctx, c, log, mgh)
	if err != nil {
		return err
	}

	agentConfigValues := &HoHAgentConfigValues{
		HoHAgentImage:        config.GetImage(mgh.GetAnnotations(), "hub_of_hubs_agent"),
		LeadHubID:            managedClusterName,
		KafkaBootstrapServer: kafkaBootstrapServer,
		KafkaCA:              kafkaCA,
	}

	tpl, err := parseNonHypershiftTemplates(nonHypershiftManifestFS)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err = tpl.ExecuteTemplate(&buf, "manifests/nonhypershift/agent", agentConfigValues); err != nil {
		return err
	}
	// log.Info("templates for agent objects", buf.String())

	agentManifests := []workv1.Manifest{}
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(&buf))
	for {
		b, err := yamlReader.Read()
		if err == io.EOF {
			buf.Reset()
			break
		}
		if err != nil {
			return err
		}
		if len(b) != 0 {
			rawJSON, err := yaml.ToJSON(b)
			if err != nil {
				return err
			}
			if string(rawJSON) != "null" {
				// log.Info("raw JSON object for agent", "json", rawJSON)
				agentManifests = append(agentManifests, workv1.Manifest{
					RawExtension: runtime.RawExtension{Raw: rawJSON},
				})
			}
		}
	}

	agentWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHAgentWorkSuffix),
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: agentManifests,
			},
		},
	}

	_, err = applyManifestWork(ctx, c, log, agentWork)
	return err
}

// applyHoHAgentHypershiftWork creates or updates multicluster-globalhub-agent manifestwork
func applyHoHAgentHypershiftWork(ctx context.Context, c client.Client, log logr.Logger,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, managedClusterName string, hcConfig *config.HostedClusterConfig,
) error {
	kafkaBootstrapServer, kafkaCA, err := getKafkaConfig(ctx, c, log, mgh)
	if err != nil {
		return err
	}

	agentConfigValues := &HoHAgentConfigValues{
		HoHAgentImage:          config.GetImage(mgh.GetAnnotations(), "hub_of_hubs_agent"),
		LeadHubID:              managedClusterName,
		KafkaBootstrapServer:   kafkaBootstrapServer,
		KafkaCA:                kafkaCA,
		HostedClusterNamespace: fmt.Sprintf("%s-%s", hcConfig.HostingNamespace, hcConfig.HostedClusterName),
	}

	tpl, err := parseAgentHypershiftTemplates(hypershiftAgentManifestFS)
	if err != nil {
		return err
	}

	var buf1 bytes.Buffer
	if err = tpl.ExecuteTemplate(&buf1, "manifests/hypershift/agent/hosted", agentConfigValues); err != nil {
		return err
	}
	// log.Info("templates for agent hosted objects", buf.String())

	agentHostedManifests := []workv1.Manifest{}
	yamlReader1 := yaml.NewYAMLReader(bufio.NewReader(&buf1))
	for {
		b, err := yamlReader1.Read()
		if err == io.EOF {
			buf1.Reset()
			break
		}
		if err != nil {
			return err
		}
		if len(b) != 0 {
			rawJSON, err := yaml.ToJSON(b)
			if err != nil {
				return err
			}
			if string(rawJSON) != "null" {
				// log.Info("raw JSON object for agent", "json", rawJSON)
				agentHostedManifests = append(agentHostedManifests,
					workv1.Manifest{RawExtension: runtime.RawExtension{Raw: rawJSON}})
			}
		}
	}

	agentHostedWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostedAgentWorkSuffix),
			Namespace: managedClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: agentHostedManifests,
			},
		},
	}

	if _, err = applyManifestWork(ctx, c, log, agentHostedWork); err != nil {
		return err
	}

	var buf2 bytes.Buffer
	if err = tpl.ExecuteTemplate(&buf2, "manifests/hypershift/agent/hosting", agentConfigValues); err != nil {
		return err
	}
	// log.Info("templates for agent hosting objects", buf.String())

	agentHostingManifests := []workv1.Manifest{}
	yamlReader2 := yaml.NewYAMLReader(bufio.NewReader(&buf2))
	for {
		b, err := yamlReader2.Read()
		if err == io.EOF {
			buf2.Reset()
			break
		}
		if err != nil {
			return err
		}
		if len(b) != 0 {
			rawJSON, err := yaml.ToJSON(b)
			if err != nil {
				return err
			}
			if string(rawJSON) != "null" {
				// log.Info("raw JSON object for hosting agent", "json", rawJSON)
				agentHostingManifests = append(agentHostingManifests,
					workv1.Manifest{RawExtension: runtime.RawExtension{Raw: rawJSON}})
			}
		}
	}

	agentHostingWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostingAgentWorkSuffix),
			Namespace: hcConfig.HostingClusterName,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: agentHostingManifests,
			},
		},
	}

	_, err = applyManifestWork(ctx, c, log, agentHostingWork)

	return err
}

// removePostponeDeleteAnnotationFromHubSubWork removes the postpone-delete
// annotation for leafhub subsctiption manifestwork
// so that the workagent can delete the manifestwork normally
func removePostponeDeleteAnnotationFromHubSubWork(ctx context.Context, c client.Client,
	managedClusterName string,
) error {
	hohHubMCHWork := &workv1.ManifestWork{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHubMCHWorkSuffix),
			Namespace: managedClusterName,
		}, hohHubMCHWork); err != nil {
		if errors.IsNotFound(err) {
			// hub mch manifestwork is deleted, try to remove the postpone-delete
			// annotation for leafhub subsctiption manifestwork
			hohHubSubWork := &workv1.ManifestWork{}
			if err := c.Get(ctx,
				types.NamespacedName{
					Name: fmt.Sprintf("%s-%s", managedClusterName,
						constants.HOHHubSubscriptionWorkSuffix),
					Namespace: managedClusterName,
				}, hohHubSubWork); err != nil {
				if errors.IsNotFound(err) {
					return nil
				} else {
					return err
				}
			}
			annotations := hohHubSubWork.GetAnnotations()
			if annotations != nil {
				delete(annotations, constants.WorkPostponeDeleteAnnotationKey)
				hohHubSubWork.SetAnnotations(annotations)
				return c.Update(ctx, hohHubSubWork)
			}
			return nil
		} else {
			return err
		}
	}
	return fmt.Errorf("Hub MCH manifestwork for managedcluster %s hasn't been deleted", managedClusterName)
}

// removeLeafHubHostingWork removes manifestwork for leafhub on hypershift hosting cluster
func removeLeafHubHostingWork(ctx context.Context, c client.Client, managedClusterName,
	hostingClusterName string,
) error {
	hohAgentMgtWork := &workv1.ManifestWork{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostingAgentWorkSuffix),
			Namespace: hostingClusterName,
		}, hohAgentMgtWork); err != nil {
		if errors.IsNotFound(err) {
			return nil
		} else {
			// Error reading the object
			return err
		}
	}
	if err := c.Delete(ctx, hohAgentMgtWork); err != nil {
		// Error deleting the object
		return err
	}

	hohHubMgtWork := &workv1.ManifestWork{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostingHubWorkSuffix),
			Namespace: hostingClusterName,
		}, hohHubMgtWork); err != nil {
		if errors.IsNotFound(err) {
			return nil
		} else {
			// Error reading the object
			return err
		}
	}

	return c.Delete(ctx, hohHubMgtWork)
}

// getKafkaConfig retrieves kafka server and CA from kafka secret
func getKafkaConfig(ctx context.Context, c client.Client, log logr.Logger, mgh *operatorv1alpha1.MultiClusterGlobalHub) (
	string, string, error,
) {
	// for local dev/test
	kafkaBootstrapServer, ok := mgh.GetAnnotations()[constants.HoHKafkaBootstrapServerKey]
	if ok && kafkaBootstrapServer != "" {
		log.Info("Kafka bootstrap server from annotation", "server", kafkaBootstrapServer, "certificate", "")
		return kafkaBootstrapServer, "", nil
	}

	kafkaSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: constants.HOHDefaultNamespace,
		Name:      mgh.Spec.Kafka.Name,
	}, kafkaSecret); err != nil {
		return "", "", err
	}

	return string(kafkaSecret.Data["bootstrap_server"]),
		base64.RawStdEncoding.EncodeToString(kafkaSecret.Data["CA"]), nil
}

// generatePullSecret copy the image pull secret to target namespace
func generatePullSecret(ctx context.Context, c client.Client, namespace string) (*corev1.Secret, error) {
	if imagePullSecretName != "" {
		imagePullSecret := &corev1.Secret{}
		if err := c.Get(ctx,
			types.NamespacedName{
				Namespace: podNamespace,
				Name:      imagePullSecretName,
			}, imagePullSecret); err != nil {
			return nil, err
		}
		return &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      imagePullSecret.Name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				".dockerconfigjson": imagePullSecret.Data[".dockerconfigjson"],
			},
			Type: corev1.SecretTypeDockerConfigJson,
		}, nil
	}
	return nil, nil
}

// applyManifestWork creates or updates a single manifestwork resource
func applyManifestWork(ctx context.Context, c client.Client, log logr.Logger, desiredWork *workv1.ManifestWork) (
	*workv1.ManifestWork, error,
) {
	existingWork := &workv1.ManifestWork{}
	if err := c.Get(ctx,
		types.NamespacedName{
			Name:      desiredWork.GetName(),
			Namespace: desiredWork.GetNamespace(),
		}, existingWork); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating manifestwork", "namespace", desiredWork.GetNamespace(), "name", desiredWork.GetName())
			if err := c.Create(ctx, desiredWork); err != nil {
				return nil, err
			}
			return desiredWork, nil
		} else {
			// Error reading the object
			return nil, err
		}
	}

	modified, err := ensureManifestWork(desiredWork, existingWork, log)
	if err != nil {
		return nil, err
	}

	if modified {
		log.Info("updating manifestwork because it is changed",
			"namespace", desiredWork.GetNamespace(), "name", desiredWork.GetName())
		desiredWork.ObjectMeta.ResourceVersion = existingWork.ObjectMeta.ResourceVersion
		if err := c.Update(ctx, desiredWork); err != nil {
			return nil, err
		}
		return desiredWork, nil
	}

	log.Info("manifestwork is not changed", "namespace", desiredWork.GetNamespace(), "name", desiredWork.GetName())
	return existingWork, nil
}

// ensureManifestWork compare desired manifestwork and existing manifeswork
// return if the desired manifeswork is modified and error if necessary
func ensureManifestWork(desired, existing *workv1.ManifestWork, log logr.Logger) (bool, error) {
	if !equality.Semantic.DeepDerivative(desired.Spec.DeleteOption, existing.Spec.DeleteOption) {
		return true, nil
	}

	if !equality.Semantic.DeepDerivative(desired.Spec.ManifestConfigs, existing.Spec.ManifestConfigs) {
		return true, nil
	}

	if len(desired.Spec.Workload.Manifests) != len(existing.Spec.Workload.Manifests) {
		return true, nil
	}

	for i, m := range existing.Spec.Workload.Manifests {
		var desiredObj, existingObj interface{}
		if len(m.RawExtension.Raw) > 0 {
			if err := json.Unmarshal(m.RawExtension.Raw, &existingObj); err != nil {
				return false, err
			}
		} else {
			existingObj = m.RawExtension.Object
		}

		if len(desired.Spec.Workload.Manifests[i].RawExtension.Raw) > 0 {
			if err := json.Unmarshal(desired.Spec.Workload.Manifests[i].RawExtension.Raw, &desiredObj); err != nil {
				return false, err
			}
		} else {
			desiredObjBytes, err := json.Marshal(desired.Spec.Workload.Manifests[i].RawExtension.Object)
			if err != nil {
				return false, err
			}
			if err := json.Unmarshal(desiredObjBytes, &desiredObj); err != nil {
				return false, err
			}
		}

		metadata := existingObj.(map[string]interface{})["metadata"].(map[string]interface{})
		// to avoid noise of compare due to https://github.com/kubernetes/kubernetes/issues/67610
		metadata["creationTimestamp"] = nil
		if !equality.Semantic.DeepDerivative(desiredObj, existingObj) {
			log.Info("existing manifest object is not equal to the desired manifest object",
				"namespace", existing.GetNamespace(), "name", existing.GetName(), "index", i)
			return true, nil
		}
	}

	return false, nil
}
