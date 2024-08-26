package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"github.com/go-logr/logr"
	"github.com/go-openapi/swag"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/clients"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	stackroxCentralServiceName string = "central"
	stackroxCentralRouteName   string = "central"
	stackroxSecretAnnotation   string = "global-hub.open-cluster-management.io/with-stackrox-credentials-secret"
)

type stackroxDataSyncer struct {
	mgr                     manager.Manager
	ticker                  *time.Ticker
	topic                   string
	producer                transport.Producer
	client                  client.Client
	stackroxCentralRequests []stackroxCentralRequest
	log                     logr.Logger
}

func (s *stackroxDataSyncer) produceToKafka(ctx context.Context, messageStruct any) error {
	version, err := version.VersionFrom("0.1")
	if err != nil {
		return fmt.Errorf("failed to get version instance: %v", err)
	}
	emitter := generic.NewGenericEmitter(enum.SecurityAlertCountsType, messageStruct, generic.WithTopic(s.topic), generic.WithVersion(version))

	evt, err := emitter.ToCloudEvent()
	if err != nil {
		return fmt.Errorf("failed to get CloudEvent instance from event %s: %v", *evt, err)
	}

	if !emitter.ShouldSend() {
		s.log.Info("should not send message to kafka", "topic", s.topic, "message", string(evt.Data()))
		return nil
	}

	s.log.Info("pushing message to kafka", "topic", s.topic, "message", string(evt.Data()))
	cloudEventsContext := cecontext.WithTopic(ctx, emitter.Topic())

	if err = s.producer.SendEvent(cloudEventsContext, *evt); err != nil {
		return fmt.Errorf("failed to send event %s: %v", *evt, err)
	}

	return nil
}

func (s *stackroxDataSyncer) fetchCentralData(ctx context.Context, central stackroxCentral) error {
	centralData := stackroxCentrals[central]

	mutex.Lock()
	defer mutex.Unlock()

	centralData.internalBaseURL = s.getCentralInternalBaseURL(central)
	externalBaseURL, err := s.getCentralExternalBaseURL(ctx, central)
	if err != nil {
		return fmt.Errorf("failed to get central external base URL (name: %s, namespace: %s): %v", central.name, central.namespace, err)
	}
	centralData.externalBaseURL = *externalBaseURL

	apiToken, err := s.getCentralAPIToken(ctx, central)
	if err != nil {
		return fmt.Errorf("failed to get central API token (name: %s, namespace: %s): %v", central.name, central.namespace, err)
	}
	centralData.apiToken = *apiToken

	return nil
}

func (s *stackroxDataSyncer) pollStackroxCentral(ctx context.Context, client clients.StackroxCentralClient, request stackroxCentralRequest, central stackroxCentral) error {
	response, statusCode, err := client.DoRequest(request.Method, request.Path, request.Body)
	if err != nil {
		return fmt.Errorf("failed to make a request to %s (method: %s, path: %s, body: %s): %v", client.BaseURL, request.Method, request.Path, request.Body, err)
	}

	if *statusCode != 200 {
		if err := s.fetchCentralData(ctx, central); err != nil {
			return fmt.Errorf("failed to fetch central data on the second attempt: %v", err)
		}
	}

	err = json.Unmarshal(response, request.CacheStruct)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response (method: %s, path: %s, body: %s): %v", request.Method, request.Path, request.Body, err)
	}

	return nil
}

func (s *stackroxDataSyncer) reconcileCentralInstance(ctx context.Context, central stackroxCentral) error {
	centralData := stackroxCentrals[central]

	for _, request := range s.stackroxCentralRequests {
		centralClient := clients.CreateStackroxCentralClient(centralData.internalBaseURL, centralData.apiToken)

		if err := s.pollStackroxCentral(ctx, centralClient, request, central); err != nil {
			return fmt.Errorf("failed to make a request to central: %v", err)
		}

		messageStruct, err := request.GenerateFromCache(request.CacheStruct, centralData.externalBaseURL)
		if err != nil {
			return fmt.Errorf("failed to generate struct for kafka message: %v", err)
		}

		if err := s.produceToKafka(ctx, messageStruct); err != nil {
			return fmt.Errorf("failed to produce a message to kafka: %v", err)
		}
	}

	return nil
}

func (s *stackroxDataSyncer) reconcile(ctx context.Context) error {
	for central, centralData := range stackroxCentrals {
		if centralData.apiToken == "" || centralData.externalBaseURL == "" || centralData.internalBaseURL == "" {
			if err := s.fetchCentralData(ctx, central); err != nil {
				return fmt.Errorf("failed to fetch central data on the first attempt (name: %s, namespace: %s): %v", central.name, central.namespace, err)
			}
		}

		if err := s.reconcileCentralInstance(ctx, central); err != nil {
			return fmt.Errorf("failed to reconcile central instance (name: %s, namespace: %s): %v", central.name, central.namespace, err)
		}
	}

	return nil
}

func (a *stackroxDataSyncer) getCentralInternalBaseURL(central stackroxCentral) string {
	url := url.URL{Scheme: "https", Host: fmt.Sprintf("%s.%s.svc.cluster.local", stackroxCentralServiceName, central.namespace)}
	return url.String()
}

func (a *stackroxDataSyncer) getCentralExternalBaseURL(ctx context.Context, central stackroxCentral) (*string, error) {
	centralRoute, err := getResource(ctx, a.client, stackroxCentralServiceName, central.namespace, &routev1.Route{}, a.log)
	if err != nil {
		return nil, err
	} else if centralRoute == nil {
		return nil, nil
	}

	centralRouteAsserted, ok := centralRoute.(*routev1.Route)
	if !ok {
		return nil, fmt.Errorf("failed to assert central route (name: %s, namespace: %s)", central.name, central.namespace)
	}

	return swag.String(centralRouteAsserted.Spec.Host), nil
}

func (a *stackroxDataSyncer) getCentralAPIToken(ctx context.Context, central stackroxCentral) (*string, error) {
	centralCRObject := &unstructured.Unstructured{}
	centralCRObject.SetGroupVersionKind(centralCRGVK)

	log := a.log.WithValues("name", central.name, "namespace", central.namespace)

	centralCR, err := getResource(ctx, a.client, central.name, central.namespace, centralCRObject, log)
	if err != nil {
		return nil, err
	} else if centralCR == nil {
		return nil, nil
	}

	annotations, found, err := unstructured.NestedStringMap(centralCRObject.Object, "metadata", "annotations")
	if err != nil {
		return nil, fmt.Errorf("failed accessing central CR annotations: %w", err)
	} else if !found {
		return nil, fmt.Errorf("annotations field in central CR not found")
	}

	stackroxSecretAnnotation, ok := annotations[stackroxSecretAnnotation]
	if !ok {
		return nil, fmt.Errorf("failed to get stackrox central API secret '%s' annotation value", stackroxSecretAnnotation)
	}

	parts := strings.Split(stackroxSecretAnnotation, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("central CR token should be of the format '<namespace/name>' where the secret for the central instance reside")
	}

	secretNamespace := parts[0]
	secretName := parts[1]

	centralSecret, err := getResource(ctx, a.client, secretName, secretNamespace, &corev1.Secret{}, log)
	if err != nil {
		return nil, err
	} else if centralSecret == nil {
		return nil, nil
	}

	centralSecretAsserted, ok := centralSecret.(*corev1.Secret)
	if !ok {
		return nil, fmt.Errorf("failed to assert central secret type")
	}

	token, ok := centralSecretAsserted.Data["token"]
	if !ok {
		return nil, fmt.Errorf("failed to get central secret data key 'token'")
	}

	return swag.String(strings.TrimSpace(string(token))), nil
}

func (a *stackroxDataSyncer) Start(ctx context.Context) error {
	a.log.Info("started stackrox data syncer...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-a.ticker.C:
				if err := a.reconcile(ctx); err != nil {
					a.log.Error(err, "failed to reconcile stackrox central data")
				}
			}
		}
	}()
	<-ctx.Done()

	a.log.Info("stopped stackrox data syncer")

	return nil
}

func AddStackroxDataSyncer(
	mgr manager.Manager,
	interval time.Duration,
	topic string,
	producer transport.Producer,
) error {
	return mgr.Add(&stackroxDataSyncer{
		mgr:                     mgr,
		client:                  mgr.GetClient(),
		ticker:                  time.NewTicker(interval),
		topic:                   topic,
		producer:                producer,
		stackroxCentralRequests: stackroxCentralRequests,
		log:                     ctrl.Log.WithName("stackrox-data-syncer"),
	})
}
