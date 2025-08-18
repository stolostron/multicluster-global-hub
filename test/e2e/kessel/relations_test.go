package kessel

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// go test ./test/e2e/kessel -v -ginkgo.focus "relations api"
var _ = Describe("relations api", Ordered, func() {
	BeforeAll(func() {
	})

	It("Create a tuples", func() {
		url := options.RelationsHTTPURL + "/api/authz/v1beta1/tuples"
		method := "POST"
		payload := []byte(`{
			"upsert": true,
			"tuples": [
					{
							"resource": {
									"id": "bob_club",
									"type": {
											"name": "group",
											"namespace": "rbac"
									}
							},
							"relation": "member",
							"subject": {
									"subject": {
											"id": "bob",
											"type": {
													"name": "principal",
													"namespace": "rbac"
											}
									}
							}
					}
			]
	}`)
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		req, err := http.NewRequest(method, url, bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Accept", "application/json")
		res, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			if err := res.Body.Close(); err != nil {
				log.Errorf("failed to close response body: %v", err)
			}
		}()

		body, err := io.ReadAll(res.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(res.StatusCode).To(Equal(200))
		Expect(string(body)).To(ContainSubstring("consistencyToken"))
	})
})
