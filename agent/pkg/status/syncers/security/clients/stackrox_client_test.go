package clients

import (
	"crypto/x509"
	"errors"
	"io"
	"mime"
	"net/http"
	"testing/iotest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/ghttp"
)

var _ = Describe("StackRox client", func() {
	Describe("Creation", func() {
		It("Can be created without the optional parameters", func() {
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL("https://my-central.com").
				SetToken("my-token").
				Build()
			Expect(client).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})

		It("Can be created without the optional CA", func() {
			pool, err := x509.SystemCertPool()
			Expect(err).ToNot(HaveOccurred())
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL("https://my-central.com").
				SetToken("my-token").
				SetCA(pool).
				Build()
			Expect(client).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})

		It("Can be created without the one optional wrapper", func() {
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL("https://my-central.com").
				SetToken("my-token").
				AddWrapper(func(t http.RoundTripper) http.RoundTripper {
					return t
				}).
				Build()
			Expect(client).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})

		It("Can be created without the two optional wrappers", func() {
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL("https://my-central.com").
				SetToken("my-token").
				AddWrapper(func(t http.RoundTripper) http.RoundTripper {
					return t
				}).
				AddWrapper(func(t http.RoundTripper) http.RoundTripper {
					return t
				}).
				Build()
			Expect(client).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})

		It("Can't be created without a logger", func() {
			client, err := NewStackRoxClient().
				SetURL("https://my-central.com").
				SetToken("my-token").
				Build()
			Expect(client).To(BeNil())
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("logger"))
			Expect(message).To(ContainSubstring("mandatory"))
		})

		It("Can't be created without an URL", func() {
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetToken("my-token").
				Build()
			Expect(client).To(BeNil())
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("URL"))
			Expect(message).To(ContainSubstring("mandatory"))
		})

		It("Can't be created without a token", func() {
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL("https://my-central.com").
				Build()
			Expect(client).To(BeNil())
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("token"))
			Expect(message).To(ContainSubstring("mandatory"))
		})
	})

	Describe("Usage", func() {
		var (
			server *Server
			client *StackRoxClient
		)

		BeforeEach(func() {
			var err error

			// Create the server:
			server = NewServer()

			// Create the client:
			client, err = NewStackRoxClient().
				SetLogger(logger).
				SetURL(server.URL()).
				SetToken("my-token").
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			server.Close()
		})

		It("Sends the header indicating that it accepts JSON", func() {
			server.AppendHandlers(
				func(w http.ResponseWriter, r *http.Request) {
					header := r.Header["Accept"]
					Expect(header).ToNot(BeNil())
					Expect(header).To(HaveLen(1))
					accept := header[0]
					media, _, err := mime.ParseMediaType(accept)
					Expect(err).ToNot(HaveOccurred())
					Expect(media).To(Equal("application/json"))
				},
			)
			_, _, err := client.DoRequest(http.MethodGet, "/my-path", "")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Sends the token in the request header", func() {
			server.AppendHandlers(
				VerifyHeaderKV("Authorization", "Bearer my-token"),
			)
			_, _, err := client.DoRequest(http.MethodGet, "/my-path", "")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Uses the requested method and path", func() {
			server.AppendHandlers(
				VerifyRequest(http.MethodGet, "/my-path"),
			)
			_, _, err := client.DoRequest(http.MethodGet, "/my-path", "")
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable(
			"Returns the status code given by the server",
			func(code int) {
				server.AppendHandlers(
					RespondWith(code, nil),
				)
				_, actual, err := client.DoRequest(http.MethodGet, "/my-path", "")
				Expect(err).ToNot(HaveOccurred())
				Expect(actual).ToNot(BeNil())
				Expect(*actual).To(Equal(code))
			},
			Entry("OK", http.StatusOK),
			Entry("Unauthorized", http.StatusUnauthorized),
			Entry("Forbidden", http.StatusForbidden),
		)

		It("Returns the response body given by the server", func() {
			server.AppendHandlers(
				RespondWith(http.StatusOK, []byte(`{
					"my-field": "my-value"
				}`)),
			)
			actual, _, err := client.DoRequest(http.MethodGet, "/my-path", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(MatchJSON(`{
				"my-field": "my-value"
			}`))
		})
	})

	Describe("Errors", func() {
		It("Returns an error if the HTTP request can't be created", func() {
			// Create a client:
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL("https://my-central.com").
				SetToken("my-token").
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Note the spaces in the method name: that is forbidden and will generate an error when the
			// request is created.
			_, _, err = client.DoRequest("G E T", "/my-path", "")
			Expect(err).To(HaveOccurred())
		})

		It("Returns an error if the HTTP response can't be read", func() {
			// Prepare the server:
			server := NewServer()
			defer server.Close()
			server.AppendHandlers(
				RespondWith(http.StatusOK, nil),
			)

			// Create a client that will replace the response body returned by the server with one that
			// returns an error when it is called.
			client, err := NewStackRoxClient().
				SetLogger(logger).
				SetURL(server.URL()).
				SetToken("my-token").
				AddWrapper(
					func(t http.RoundTripper) http.RoundTripper {
						return RoundTripperFunc(
							func(q *http.Request) (p *http.Response, err error) {
								p, err = t.RoundTrip(q)
								Expect(err).ToNot(HaveOccurred())
								p.Body = io.NopCloser(
									iotest.ErrReader(errors.New("my-error")),
								)
								return
							},
						)
					},
				).
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Check that the error is returned:
			_, _, err = client.DoRequest(http.MethodGet, "/my-path", "")
			Expect(err).To(HaveOccurred())
			message := err.Error()
			Expect(message).To(ContainSubstring("my-error"))
		})
	})
})
