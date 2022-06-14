package tests

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

var _ = Describe("Client", Label("connection"), func() {

	It("kubeclient", func() {
		hubClient := clients.KubeClient()
		deployClient := hubClient.AppsV1().Deployments(testOptions.HubCluster.Namespace)
		deployList, err := deployClient.List(context.TODO(), metav1.ListOptions{Limit: 2})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(deployList.Items) > 0).To(BeTrue())
	})

	It("kubeclient dynamic", func() {
		dynamicClient := clients.KubeDynamicClient()
		routeGvr := utils.NewRouteGVR()
		routeList, err := dynamicClient.Resource(routeGvr).Namespace(testOptions.HubCluster.Namespace).List(context.TODO(), metav1.ListOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(routeList.Items) > 0).To(BeTrue())
		routeBytes, err := json.MarshalIndent(routeList, "", "  ")
		Expect(err).ShouldNot(HaveOccurred())
		klog.V(6).Info(string(routeBytes))
	})

	It("non-k8s-api", func() {
		token, err := utils.FetchBearerToken(testOptions)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(token) > 0).Should(BeTrue())

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Timeout: time.Second * 10, Transport: tr}
		identityUrl := testOptions.HubCluster.MasterURL + "/apis/user.openshift.io/v1/users/~"

		req, err := http.NewRequest("GET", identityUrl, nil)
		Expect(err).ShouldNot(HaveOccurred())
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := client.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		Expect(err).ShouldNot(HaveOccurred())

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		Expect(result["metadata"].(map[string]interface{})["name"]).To(BeElementOf("kube:admin"))
	})
})
