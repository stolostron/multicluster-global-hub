package security

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/ghttp"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crfakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("StackRox syncer", func() {
	Describe("Creation", func() {
		var (
			producer transport.Producer
			client   crclient.Client
		)

		BeforeEach(func() {
			// Create a mock producer:
			producer = &transport.ProducerMock{}

			// Create a mock Kubernetes API client:
			client = crfakeclient.NewClientBuilder().Build()
		})

		It("Can be created with mandatory parameters", func() {
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetTopic("my-topic").
				SetProducer(producer).
				SetKubernetesClient(client).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(syncer).ToNot(BeNil())
		})

		It("Can be created with optional poll interval", func() {
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetTopic("my-topic").
				SetProducer(producer).
				SetKubernetesClient(client).
				SetPollInterval(10 * time.Minute).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(syncer).ToNot(BeNil())
		})

		It("Can't be created without a logger", func() {
			syncer, err := NewStackRoxSyncer().
				SetTopic("my-topic").
				SetProducer(producer).
				SetKubernetesClient(client).
				Build()
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("logger"))
			Expect(message).To(ContainSubstring("mandatory"))
			Expect(syncer).To(BeNil())
		})

		It("Can't be created without a topic", func() {
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetProducer(producer).
				SetKubernetesClient(client).
				Build()
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("topic"))
			Expect(message).To(ContainSubstring("mandatory"))
			Expect(syncer).To(BeNil())
		})

		It("Can't be created without a producer", func() {
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetTopic("my-topic").
				SetKubernetesClient(client).
				Build()
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("producer"))
			Expect(message).To(ContainSubstring("mandatory"))
			Expect(syncer).To(BeNil())
		})

		It("Can't be created without a Kubernetes API client", func() {
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetTopic("my-topic").
				SetProducer(producer).
				Build()
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("client"))
			Expect(message).To(ContainSubstring("mandatory"))
			Expect(syncer).To(BeNil())
		})

		It("Can't be created with negative time interval", func() {
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetTopic("my-topic").
				SetProducer(producer).
				SetKubernetesClient(client).
				SetPollInterval(-1 * time.Minute).
				Build()
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("interval"))
			Expect(message).To(ContainSubstring("zero"))
			Expect(message).To(ContainSubstring("-1"))
			Expect(syncer).To(BeNil())
		})
	})

	Describe("Usage", func() {
		var (
			ctx    context.Context
			client crclient.Client
		)

		BeforeEach(func() {
			var err error

			// Create a context:
			ctx = context.Background()

			// Create the scheme and register the types we need:
			scheme := runtime.NewScheme()
			err = corev1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())
			err = routev1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			// Create the fake Kubernetes API client:
			client = crfakeclient.NewClientBuilder().
				WithScheme(scheme).
				Build()
		})

		It("Can be started", func() {
			// Create the producer:
			producer := &transport.ProducerMock{}

			// Create the syncer:
			syncer, err := NewStackRoxSyncer().
				SetLogger(logger).
				SetTopic("my-topic").
				SetProducer(producer).
				SetKubernetesClient(client).
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Start the syncer:
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() {
				defer GinkgoRecover()
				err := syncer.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
			}()
		})

		Context("With configured central", func() {
			var server *Server

			// RespondOK sends a valid response.
			RespondOK := RespondWith(
				http.StatusOK,
				`{
					"groups": [{
						"group": "",
						"counts": [
							{
								"severity": "LOW_SEVERITY",
								"count": "1"
							},
							{
								"severity": "MEDIUM_SEVERITY",
								"count": "2"
							},
							{
								"severity": "HIGH_SEVERITY",
								"count": "3"
							},
							{
								"severity": "CRITICAL_SEVERITY",
								"count": "4"
							}
						]
					}]
				}`,
				http.Header{
					"Content-Type": []string{"application/json"},
				},
			)

			// RespondUnathorized responds with an authorization error.
			RespondUnathorized := RespondWith(http.StatusUnauthorized, nil)

			// RespondInternalError respond with an internal error.
			RespondInternalError := RespondWith(http.StatusInternalServerError, nil)

			BeforeEach(func() {
				var err error

				// Create the server:
				server = NewTLSServer()
				caCert := server.HTTPTestServer.Certificate()
				caBlock := &pem.Block{
					Type:  "CERTIFICATE",
					Bytes: caCert.Raw,
				}
				caBuffer := &bytes.Buffer{}
				err = pem.Encode(caBuffer, caBlock)
				Expect(err).ToNot(HaveOccurred())
				caBytes := caBuffer.Bytes()

				// Create the objects:
				central := &unstructured.Unstructured{}
				central.SetGroupVersionKind(centralCRGVK)
				central.SetNamespace("rhacs-operator")
				central.SetName("stackrox-central-services")
				central.SetAnnotations(map[string]string{
					stacRoxDetailsAnnotation: "multicluster-global-hub-agent/stackrox-details",
				})
				err = client.Create(ctx, central)
				Expect(err).ToNot(HaveOccurred())

				// Create the route:
				route := &routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "rhacs-operator",
						Name:      "central",
					},
					Spec: routev1.RouteSpec{
						Host: "my-console.com",
					},
				}
				err = client.Create(ctx, route)
				Expect(err).ToNot(HaveOccurred())

				// Create the secret:
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "multicluster-global-hub-agent",
						Name:      "stackrox-details",
					},
					Data: map[string][]byte{
						"url":   []byte(server.URL()),
						"token": []byte("my-token"),
						"ca":    caBytes,
					},
				}
				err = client.Create(ctx, secret)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				// Stop the server:
				server.Close()
			})

			It("Synchronizes correctly", func() {
				var err error

				// Prepare the server:
				server.AppendHandlers(
					CombineHandlers(
						VerifyHeaderKV("Authorization", "Bearer my-token"),
						VerifyRequest(http.MethodGet, "/v1/alerts/summary/counts"),
						RespondOK,
					),
				)

				// Create the producer:
				producer := &transport.ProducerMock{
					SendEventFunc: func(ctx context.Context, evt cloudevents.Event) error {
						defer GinkgoRecover()

						// Verify the message:
						Expect(evt.Data()).To(MatchJSON(`{
							"low": 1,
							"medium": 2,
							"high": 3,
							"critical": 4,
							"detail_url": "https://my-console.com/main/violations",
							"source": "rhacs-operator/stackrox-central-services"
						}`))

						return nil
					},
					ReconnectFunc: func(config *transport.TransportConfig) error {
						return nil
					},
				}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Polls in a loop", func() {
				var err error

				// Prepare the server so that it counts the requests:
				requests := &atomic.Int32{}
				server.RouteToHandler(
					http.MethodGet,
					"/v1/alerts/summary/counts",
					CombineHandlers(
						func(w http.ResponseWriter, r *http.Request) {
							requests.Add(1)
						},
						RespondOK,
					),
				)

				// Create the producer that counts the messages sent:
				messages := &atomic.Int32{}
				producer := &transport.ProducerMock{
					SendEventFunc: func(ctx context.Context, evt cloudevents.Event) error {
						messages.Add(1)
						return nil
					},
					ReconnectFunc: func(config *transport.TransportConfig) error {
						return nil
					},
				}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					SetPollInterval(1 * time.Millisecond).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Start the syncer:
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				go func() {
					defer GinkgoRecover()
					err := syncer.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				// Register the central:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())

				// Wait till the syncer has sent multiple requests and produced multiple messages:
				Eventually(func(g Gomega) {
					g.Expect(requests.Load()).To(BeNumerically(">=", 2))
					g.Expect(messages.Load()).To(BeNumerically(">=", 2))
				}).Should(Succeed())
			})

			It("Keeps polling after a failure", func() {
				var err error

				// Prepare the server so that it responds correctly for the first request, but fails
				// for the second, correctly for the third, fails for the fourth, so on.
				count := &atomic.Int32{}
				server.RouteToHandler(
					http.MethodGet,
					"/v1/alerts/summary/counts",
					func(w http.ResponseWriter, r *http.Request) {
						if count.Load()%2 == 0 {
							RespondOK(w, r)
						} else {
							RespondInternalError(w, r)
						}
						count.Add(1)
					},
				)

				// Create a producer that counts the messages:
				messages := &atomic.Int32{}
				producer := &transport.ProducerMock{
					SendEventFunc: func(ctx context.Context, evt cloudevents.Event) error {
						messages.Add(1)
						return nil
					},
					ReconnectFunc: func(config *transport.TransportConfig) error {
						return nil
					},
				}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					SetPollInterval(1 * time.Millisecond).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Start the syncer:
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				go func() {
					defer GinkgoRecover()
					err := syncer.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				// Register the central:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())

				// Wait till the syncer has produced at least two messages, as that means that it
				// tolerated a least one failure.
				Eventually(func(g Gomega) {
					g.Expect(messages.Load()).To(BeNumerically(">=", 2))
				}).Should(Succeed())
			})

			It("Computes default API URL", func() {
				var err error

				// Remove the URL key from the secret:
				secretObject := &corev1.Secret{}
				secretKey := types.NamespacedName{
					Namespace: "multicluster-global-hub-agent",
					Name:      "stackrox-details",
				}
				err = client.Get(ctx, secretKey, secretObject)
				Expect(err).ToNot(HaveOccurred())
				delete(secretObject.Data, "url")
				err = client.Update(ctx, secretObject)
				Expect(err).ToNot(HaveOccurred())

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("https://central.rhacs-operator.svc"))
			})

			It("Fails if there is no token", func() {
				var err error

				// Remove the token from the secret:
				secretObject := &corev1.Secret{}
				secretKey := types.NamespacedName{
					Namespace: "multicluster-global-hub-agent",
					Name:      "stackrox-details",
				}
				err = client.Get(ctx, secretKey, secretObject)
				Expect(err).ToNot(HaveOccurred())
				delete(secretObject.Data, "token")
				err = client.Update(ctx, secretObject)
				Expect(err).ToNot(HaveOccurred())

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("token"))
				Expect(message).To(ContainSubstring("mandatory"))
			})

			It("Fails if there is no CA", func() {
				var err error

				// Remove the URL from the secret:
				secretObject := &corev1.Secret{}
				secretKey := types.NamespacedName{
					Namespace: "multicluster-global-hub-agent",
					Name:      "stackrox-details",
				}
				err = client.Get(ctx, secretKey, secretObject)
				Expect(err).ToNot(HaveOccurred())
				delete(secretObject.Data, "ca")
				err = client.Update(ctx, secretObject)
				Expect(err).ToNot(HaveOccurred())

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("x509"))
				Expect(message).To(ContainSubstring("unknown"))
				Expect(message).To(ContainSubstring("authority"))
			})

			It("Refreshes the connection details after failure", func() {
				var err error

				// Prepare the server so that the first time will reject the request and update the
				// secret, and so that the second time it will accept the request.
				server.AppendHandlers(
					CombineHandlers(
						VerifyHeaderKV("Authorization", "Bearer my-token"),
						func(w http.ResponseWriter, r *http.Request) {
							secretObject := &corev1.Secret{}
							secretKey := types.NamespacedName{
								Namespace: "multicluster-global-hub-agent",
								Name:      "stackrox-details",
							}
							err = client.Get(ctx, secretKey, secretObject)
							Expect(err).ToNot(HaveOccurred())
							secretObject.Data["token"] = []byte("new-token")
							err = client.Update(ctx, secretObject)
							Expect(err).ToNot(HaveOccurred())
						},
						RespondUnathorized,
					),
					CombineHandlers(
						VerifyHeaderKV("Authorization", "Bearer new-token"),
						RespondOK,
					),
				)

				// Create the producer:
				producer := &transport.ProducerMock{
					SendEventFunc: func(ctx context.Context, evt cloudevents.Event) error {
						defer GinkgoRecover()

						// Verify the message:
						Expect(evt.Data()).To(MatchJSON(`{
							"low": 1,
							"medium": 2,
							"high": 3,
							"critical": 4,
							"detail_url": "https://my-console.com/main/violations",
							"source": "rhacs-operator/stackrox-central-services"
						}`))

						return nil
					},
					ReconnectFunc: func(config *transport.TransportConfig) error {
						return nil
					},
				}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Fails if not registered", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("isn't registered"))
			})

			It("Fails if unregistered", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Unregister(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("isn't registered"))
			})

			It("Fails if central doesn't exist", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Delete the central:
				centralObject := &unstructured.Unstructured{}
				centralObject.SetGroupVersionKind(centralCRGVK)
				centralObject.SetNamespace("rhacs-operator")
				centralObject.SetName("stackrox-central-services")
				err = client.Delete(ctx, centralObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := crclient.ObjectKeyFromObject(centralObject)
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("central"))
				Expect(message).To(ContainSubstring("not found"))
			})

			It("Fails if the annotation isn't present", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Delete the annotation:
				centralObject := &unstructured.Unstructured{}
				centralObject.SetGroupVersionKind(centralCRGVK)
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = client.Get(ctx, centralKey, centralObject)
				Expect(err).ToNot(HaveOccurred())
				annotations := centralObject.GetAnnotations()
				delete(annotations, stacRoxDetailsAnnotation)
				centralObject.SetAnnotations(annotations)
				err = client.Update(ctx, centralObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("doesn't have"))
				Expect(message).To(ContainSubstring("annotation"))
			})

			It("Fails if the annotation isn't well formed", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Put an incorrect value in the annotation:
				centralObject := &unstructured.Unstructured{}
				centralObject.SetGroupVersionKind(centralCRGVK)
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = client.Get(ctx, centralKey, centralObject)
				Expect(err).ToNot(HaveOccurred())
				annotations := centralObject.GetAnnotations()
				annotations[stacRoxDetailsAnnotation] = "junk"
				centralObject.SetAnnotations(annotations)
				err = client.Update(ctx, centralObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("annotation"))
				Expect(message).To(ContainSubstring("not valid"))
				Expect(message).To(ContainSubstring("junk"))
			})

			It("Fails if the CA isn't well formed", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Put an incorrect value in the CA:
				secretObject := &corev1.Secret{}
				secretKey := types.NamespacedName{
					Namespace: "multicluster-global-hub-agent",
					Name:      "stackrox-details",
				}
				err = client.Get(ctx, secretKey, secretObject)
				Expect(err).ToNot(HaveOccurred())
				secretObject.Data["ca"] = []byte("junk")
				err = client.Update(ctx, secretObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("doesn't contain"))
				Expect(message).To(ContainSubstring("certificate"))
			})

			It("Fails if the secret doesn't exist", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Delete the secret:
				secretObject := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "multicluster-global-hub-agent",
						Name:      "stackrox-details",
					},
				}
				err = client.Delete(ctx, secretObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("secret"))
				Expect(message).To(ContainSubstring("not found"))
			})

			It("Fails if the route doesn't exist", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Delete the route:
				routeObject := &routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "rhacs-operator",
						Name:      "central",
					},
				}
				err = client.Delete(ctx, routeObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("route"))
				Expect(message).To(ContainSubstring("not found"))
			})

			It("Fails if the route doesn't have a host", func() {
				var err error

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Remove the host from the route:
				routeObject := &routev1.Route{}
				routeKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "central",
				}
				err = client.Get(ctx, routeKey, routeObject)
				Expect(err).ToNot(HaveOccurred())
				routeObject.Spec.Host = ""
				err = client.Update(ctx, routeObject)
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("route"))
				Expect(message).To(ContainSubstring("doesn't have"))
				Expect(message).To(ContainSubstring("host"))
			})

			It("Fails if server returns invalid JSON", func() {
				var err error

				// Prepare the server so that the first time will reject the request and update the
				// secret, and so that the second time it will accept the request.
				server.AppendHandlers(
					RespondWith(http.StatusOK, "junk"),
				)

				// Create the producer:
				producer := &transport.ProducerMock{}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("invalid character 'j'"))
			})

			It("Fails if the producer fails", func() {
				var err error

				// Prepare the server:
				server.AppendHandlers(RespondOK)

				// Create a producer that fails:
				producer := &transport.ProducerMock{
					SendEventFunc: func(ctx context.Context, evt cloudevents.Event) error {
						return errors.New("failed to produce")
					},
					ReconnectFunc: func(config *transport.TransportConfig) error {
						return nil
					},
				}

				// Create the syncer:
				syncer, err := NewStackRoxSyncer().
					SetLogger(logger).
					SetTopic("my-topic").
					SetProducer(producer).
					SetKubernetesClient(client).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Try to synchronize:
				centralKey := types.NamespacedName{
					Namespace: "rhacs-operator",
					Name:      "stackrox-central-services",
				}
				err = syncer.Register(ctx, centralKey)
				Expect(err).ToNot(HaveOccurred())
				err = syncer.Sync(ctx, centralKey)
				Expect(err).To(HaveOccurred())
				message := err.Error()
				Expect(message).To(ContainSubstring("failed to produce"))
			})
		})
	})
})
