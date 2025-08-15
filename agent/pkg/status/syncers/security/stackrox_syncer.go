package security

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	routev1 "github.com/openshift/api/route/v1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crmanager "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/security/clients"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	zaplogger "github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	stackRoxServiceName         = "central"
	stackRoxRouteName           = "central"
	stacRoxDetailsAnnotation    = "global-hub.open-cluster-management.io/with-stackrox-credentials-secret"
	StackRoxDefaultPollInterval = 30 * time.Minute
)

// StackRoxSyncerBuilder contains the data and logic needed to create a new StackRox syncer. Don't create instances of
// this type directly, use the NewStackRoxSyncer function instead.
type StackRoxSyncerBuilder struct {
	logger       *zap.SugaredLogger
	topic        string
	producer     transport.Producer
	kubeClient   crclient.Client
	pollInterval time.Duration
}

// StackRoxSyncer knows how to pull multiple StackRox API servers to extract information and send it to Kafka. Don't
// create instances of this type directly, use the NewStackRoxSyncer function instead.
type StackRoxSyncer struct {
	logger         *zap.SugaredLogger
	topic          string
	producer       transport.Producer
	kubeClient     crclient.Client
	pollInterval   time.Duration
	dataLock       *sync.Mutex
	dataMap        map[types.NamespacedName]*stackRoxData
	requests       []stackRoxRequest
	currentVersion *eventversion.Version
	lastSentData   any
}

// stackRoxConnDetails contains the details of the StackRox central.
type stackRoxConnDetails struct {
	apiURL   string
	apiToken string
	caPool   *x509.CertPool
}

// stackRoxData contains the information relative to a StackRox central instance that has been registered for
// synchronization.
type stackRoxData struct {
	lock       *sync.Mutex
	key        types.NamespacedName
	apiClient  *clients.StackRoxClient
	consoleURL string
}

// NewStackRoxSyncer creates a builder that can then be used to configure and create a new StackRox syncer.
func NewStackRoxSyncer() *StackRoxSyncerBuilder {
	return &StackRoxSyncerBuilder{
		pollInterval: StackRoxDefaultPollInterval,
	}
}

// SetLogger sets the logger that the synceer will use to write to the log. This is mandatory.
func (b *StackRoxSyncerBuilder) SetLogger(value *zap.SugaredLogger) *StackRoxSyncerBuilder {
	b.logger = value
	return b
}

// SetTopic sets the name of the Kafka topic where the syncer will publish the messages. This is mandatory.
func (b *StackRoxSyncerBuilder) SetTopic(value string) *StackRoxSyncerBuilder {
	b.topic = value
	return b
}

// SetProducer sets the producer that the syncer will use to send the messages to Kafka. This is mandatory.
func (b *StackRoxSyncerBuilder) SetProducer(value transport.Producer) *StackRoxSyncerBuilder {
	b.producer = value
	return b
}

// SetKubernetesClient sets the Kubernetes API client that the syncer will use to read the StackRox central object and
// secrets that contain the connection details. This is mandatory.
func (b *StackRoxSyncerBuilder) SetKubernetesClient(value crclient.Client) *StackRoxSyncerBuilder {
	b.kubeClient = value
	return b
}

// SetPollInterval sets the polling interval. Default value is 30 minutes.
func (b *StackRoxSyncerBuilder) SetPollInterval(value time.Duration) *StackRoxSyncerBuilder {
	b.pollInterval = value
	return b
}

// Build uses the information stored in the builder to create a new StackRox syncer.
func (b *StackRoxSyncerBuilder) Build() (result *StackRoxSyncer, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.topic == "" {
		err = errors.New("topic is mandatory")
		return
	}
	if b.producer == nil {
		err = errors.New("producer is mandatory")
		return
	}
	if b.kubeClient == nil {
		err = errors.New("client is mandatory")
		return
	}
	if b.pollInterval <= 0 {
		err = fmt.Errorf(
			"poll interval must be greater than zero, but it is %s",
			b.pollInterval,
		)
		return
	}

	// Create and populate the object:
	result = &StackRoxSyncer{
		logger:         b.logger,
		topic:          b.topic,
		producer:       b.producer,
		kubeClient:     b.kubeClient,
		pollInterval:   b.pollInterval,
		dataLock:       &sync.Mutex{},
		dataMap:        map[types.NamespacedName]*stackRoxData{},
		requests:       stackRoxRequests,
		currentVersion: eventversion.NewVersion(),
	}
	return
}

// Register registers a new StackRox central instance, so that it will eventually be synchronized. If it is already
// registered then this does nothing.
func (s *StackRoxSyncer) Register(ctx context.Context, key types.NamespacedName) error {
	s.dataLock.Lock()
	defer s.dataLock.Unlock()

	_, ok := s.dataMap[key]
	if !ok {
		s.dataMap[key] = &stackRoxData{
			lock: &sync.Mutex{},
			key:  key,
		}
		s.logger.Info(
			"Registered central",
			"namespace", key.Namespace,
			"name", key.Name,
		)
	}
	return nil
}

// Unregister unregisters a StackRox central instance, so that it will no longer be synchronized. If it wasn't
// registered then it does nothing.
func (s *StackRoxSyncer) Unregister(ctx context.Context, key types.NamespacedName) error {
	s.dataLock.Lock()
	defer s.dataLock.Unlock()

	_, ok := s.dataMap[key]
	if ok {
		delete(s.dataMap, key)
		s.logger.Info(
			"Unregistered central",
			"namespace", key.Namespace,
			"name", key.Name,
		)
	}
	return nil
}

// Sync performs the synchronization of a StackRox central with the given namespace and name. There is usually no need
// to do this explicitly, as it will be done periodically, but it is convenient for unit tests.
func (s *StackRoxSyncer) Sync(ctx context.Context, key types.NamespacedName) error {
	var err error

	// Check if the data has been registered. Note that we don't need to acquire the lock here because this is a
	// read only operation and it is safe to do that concurrently.
	data, ok := s.dataMap[key]
	if !ok {
		return fmt.Errorf(
			"central '%s/%s' isn't registered",
			key.Namespace, key.Name,
		)
	}

	// For the rest of the synchronization process we need to be sure that no one else is using the data.
	data.lock.Lock()
	defer data.lock.Unlock()

	// If this is the first time that this data is used then the API client won't be created yet, so we need to
	// refresh the data.
	if data.apiClient == nil {
		err = s.refresh(ctx, data)
		if err != nil {
			return err
		}
	}

	// If we are here then we have everthing that is needed to perform the actual synchronization:
	err = s.sync(ctx, data)
	if err != nil {
		return err
	}

	return nil
}

// refresh refreshes the given data, fetching the connection details, creating the API client and fetching the console
// URL.
func (s *StackRoxSyncer) refresh(ctx context.Context, data *stackRoxData) error {
	// Create a logger with the namespace and name:
	logger := s.logger.With(
		"namespace", data.key.Namespace,
		"name", data.key.Name,
	)

	// Fetch the connection details:
	connDetails, err := s.fetchConnDetails(ctx, data.key)
	if err != nil {
		return err
	}
	logger.Info(
		"Got central connection details",
		"url", connDetails.apiURL,
		"token", fmt.Sprintf("%0.3s...", connDetails.apiToken),
		"ca", connDetails.caPool != nil,
	)

	// Create the API client:
	apiClient, err := clients.NewStackRoxClient().
		SetLogger(logger).
		SetURL(connDetails.apiURL).
		SetToken(connDetails.apiToken).
		SetCA(connDetails.caPool).
		Build()
	if err != nil {
		return err
	}
	logger.Info(
		"Created central API client",
		"url", connDetails.apiURL,
		"token", fmt.Sprintf("%0.3s...", connDetails.apiToken),
		"ca", connDetails.caPool != nil,
	)

	// Fetch the console URL:
	consoleURL, err := s.fetchConsoleURL(ctx, data.key)
	if err != nil {
		return err
	}
	logger.Info(
		"Got central console URL",
		"url", consoleURL,
	)

	// Now that everything suceeded we can update the data:
	data.apiClient = apiClient
	data.consoleURL = consoleURL
	return nil
}

// fetchConnDetails fetches the connection details for the StackRox central instance with the given namespace and name.
func (s *StackRoxSyncer) fetchConnDetails(ctx context.Context, centralKey types.NamespacedName,
) (result *stackRoxConnDetails, err error) {
	// Try to fetch the centralObject:
	centralObject := &unstructured.Unstructured{}
	centralObject.SetGroupVersionKind(centralCRGVK)
	err = s.kubeClient.Get(ctx, centralKey, centralObject)
	if err != nil {
		return
	}

	// Get the location of the secret containing the connection details:
	value := centralObject.GetAnnotations()[stacRoxDetailsAnnotation]
	if value == "" {
		err = fmt.Errorf(
			"central '%s/%s' doesn't have the '%s' annotation",
			centralKey.Namespace, centralKey.Name, stacRoxDetailsAnnotation,
		)
		return
	}

	// The value of the annotation should be a namespace followed by a forward slash and then a secret name:
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		err = fmt.Errorf(
			"value of annotation '%s' is not valid, should be a namespace followed "+
				"by a forward slash and a secret name, but it is '%s'",
			stacRoxDetailsAnnotation, value,
		)
		return
	}

	// Try to fetch the secret:
	secretObject := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: parts[0],
		Name:      parts[1],
	}
	err = s.kubeClient.Get(ctx, secretKey, secretObject)
	if err != nil {
		return
	}

	// Get the bytes of the keys that we need:
	var apiURLBytes, apiTokenBytes, caPoolBytes []byte
	if secretObject.Data != nil {
		apiURLBytes = secretObject.Data["url"]
		apiTokenBytes = secretObject.Data["token"]
		caPoolBytes = secretObject.Data["ca"]
	}

	// Get the URL:
	var apiURL string
	if apiURLBytes != nil {
		apiURL = strings.TrimSpace(string(apiURLBytes))
	} else {
		apiURL = fmt.Sprintf(
			"https://%s.%s.svc",
			stackRoxServiceName, centralKey.Namespace,
		)
	}

	// Get the apiToken:
	var apiToken string
	if apiTokenBytes != nil {
		apiToken = strings.TrimSpace(string(apiTokenBytes))
	} else {
		err = fmt.Errorf(
			"connection details secret '%s/%s' doesn't contain the mandatory "+
				"'token' key",
			secretKey.Namespace, secretKey.Name,
		)
		return
	}

	// Get the CA:
	var caPool *x509.CertPool
	if caPoolBytes != nil {
		caPool = x509.NewCertPool()
		ok := caPool.AppendCertsFromPEM(caPoolBytes)
		if !ok {
			err = fmt.Errorf(
				"key 'ca' of connection details secret '%s/%s' doesn't contain any CA certificate",
				secretKey.Namespace, secretKey.Name,
			)
			return
		}
	}

	// Return the result:
	result = &stackRoxConnDetails{
		apiURL:   apiURL,
		apiToken: apiToken,
		caPool:   caPool,
	}
	return
}

// fetchConsoleURL fetches the conole URL of the given StackRox central. If the route doesn't exist, or the host hasn't
// been populated yet, it returns an empty string and no error.
func (s *StackRoxSyncer) fetchConsoleURL(ctx context.Context, centralKey types.NamespacedName,
) (result string, err error) {
	// Try to fetch the route:
	routeObject := &routev1.Route{}
	routeKey := types.NamespacedName{
		Namespace: centralKey.Namespace,
		Name:      stackRoxRouteName,
	}
	err = s.kubeClient.Get(ctx, routeKey, routeObject)
	if err != nil {
		return
	}

	// Check if the host is already populated:
	if routeObject.Spec.Host == "" {
		err = fmt.Errorf(
			"route '%s/%s' doesn't have a host",
			routeKey.Namespace, routeKey.Name,
		)
		return
	}

	// Calculate the console URL from the host of the route:
	result = fmt.Sprintf("https://%s", routeObject.Spec.Host)
	return
}

func (s *StackRoxSyncer) sync(ctx context.Context, data *stackRoxData) error {
	for _, request := range s.requests {
		if err := s.poll(ctx, data, request); err != nil {
			return fmt.Errorf("failed to make a request to central: %v", err)
		}

		messageStruct, err := request.GenerateFromCache(
			request.CacheStruct, data.consoleURL, data.key.Namespace, data.key.Name)
		if err != nil {
			return fmt.Errorf("failed to generate struct for kafka message: %v", err)
		}

		// If the payload not updated since the previous one, then don't need sync it again
		// if equality.Semantic.DeepEqual(messageStruct, s.lastSentData) {
		// 	return nil
		// }

		s.currentVersion.Incr()
		if err := s.produce(ctx, messageStruct); err != nil {
			return fmt.Errorf("failed to produce a message to kafka: %v", err)
		}
		s.lastSentData = messageStruct
	}

	return nil
}

func (s *StackRoxSyncer) produce(ctx context.Context, messageStruct any) error {
	s.dataLock.Lock()
	defer s.dataLock.Unlock()

	evt := ToEvent(configs.GetLeafHubName(), string(enum.SecurityAlertCountsType), s.currentVersion.String())
	err := evt.SetData(cloudevents.ApplicationJSON, messageStruct)
	if err != nil {
		return fmt.Errorf("failed to get CloudEvent instance from event %s: %v", *evt, err)
	}

	s.logger.Info("pushing message to kafka", "topic", s.topic, "message", string(evt.Data()))
	cloudEventsContext := cecontext.WithTopic(ctx, s.topic)

	if err = s.producer.SendEvent(cloudEventsContext, *evt); err != nil {
		return fmt.Errorf("failed to send event %s: %v", *evt, err)
	}

	s.currentVersion.Next()

	return nil
}

func (s *StackRoxSyncer) poll(ctx context.Context, data *stackRoxData, request stackRoxRequest) error {
	response, status, err := data.apiClient.DoRequest(request.Method, request.Path, request.Body)
	if err != nil {
		return err
	}

	// If the request fails with an error related to authentication, then we refresh the data and try again, only
	// once:
	if status != nil && *status == http.StatusUnauthorized || *status == http.StatusForbidden {
		err = s.refresh(ctx, data)
		if err != nil {
			return err
		}
		response, status, err = data.apiClient.DoRequest(request.Method, request.Path, request.Body)
		if err != nil {
			return err
		}
		if status != nil && *status != http.StatusOK {
			return fmt.Errorf("request failed with status code %d", *status)
		}
	}

	err = json.Unmarshal(response, request.CacheStruct)
	if err != nil {
		return fmt.Errorf(
			"failed to unmarshal response (method: %s, path: %s, body: %s): %v",
			request.Method, request.Path, request.Body, err,
		)
	}

	return nil
}

func (s *StackRoxSyncer) Start(ctx context.Context) error {
	s.logger.Info("Starting syncer")
	ticker := time.NewTicker(s.pollInterval)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopped syncer")
			return nil
		case <-ticker.C:
			func() {
				s.dataLock.Lock()
				defer s.dataLock.Unlock()
				for key := range s.dataMap {
					go func() {
						err := s.Sync(ctx, key)
						if err != nil {
							s.logger.Error(
								err,
								"Failed to synchronize",
								"namespace", key.Namespace,
								"name", key.Name,
							)
						}
					}()
				}
			}()
		}
	}
}

func AddStackroxDataSyncer(
	manager crmanager.Manager,
	interval time.Duration,
	topic string,
	producer transport.Producer,
) error {
	syncer, err := NewStackRoxSyncer().
		SetLogger(zaplogger.ZapLogger("stackrox-syncer")).
		SetTopic(topic).
		SetProducer(producer).
		SetKubernetesClient(manager.GetClient()).
		Build()
	if err != nil {
		return err
	}
	return manager.Add(syncer)
}

func ToEvent(source string, eventType string, currentVersion string) *cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetSource(source)
	e.SetType(eventType)
	e.SetExtension(eventversion.ExtVersion, currentVersion)
	return &e
}
