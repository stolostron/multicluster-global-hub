package tests

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-global-hub/test/pkg/utils"
)

var _ = Describe("Check all the clients could connect to the HoH servers", Label("e2e-tests-connection"), func() {
	It("connect to the apiserver with kubernetes interface", func() {
		hubClient := clients.KubeClient()
		deployClient := hubClient.AppsV1().Deployments(testOptions.HubCluster.Namespace)
		deployList, err := deployClient.List(context.TODO(), metav1.ListOptions{Limit: 2})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(deployList.Items) > 0).To(BeTrue())
	})

	It("connect to the apiserver with dynamic interface", func() {
		dynamicClient := clients.KubeDynamicClient()
		hohConfigMapGVR := utils.NewHoHConfigMapGVR()
		configMapList, err := dynamicClient.Resource(hohConfigMapGVR).Namespace(
			"open-cluster-management-global-hub-system").List(context.TODO(), metav1.ListOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(configMapList.Items) > 0).To(BeTrue())
	})

	It("check whether the cluster is running properly", func() {
		hubClient := clients.KubeClient()
		healthy, err := hubClient.Discovery().RESTClient().Get().AbsPath("/healthz").DoRaw(context.TODO())
		Expect(err).ShouldNot(HaveOccurred())
		Expect(string(healthy)).To(Equal("ok"))
	})

	It("connect to the nonk8s-server with specific user", func() {
		token, err := utils.FetchBearerToken(testOptions)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(token) > 0).Should(BeTrue())

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Timeout: time.Second * 10, Transport: tr}

		identityUrl := testOptions.HubCluster.ApiServer + "/apis/user.openshift.io/v1/users/~"

		req, err := http.NewRequest("GET", identityUrl, nil)
		Expect(err).ShouldNot(HaveOccurred())
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := client.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).ShouldNot(HaveOccurred())

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		userRes, _ := json.MarshalIndent(result, "", "  ")
		klog.V(6).Info(fmt.Sprintf("The Test User Infomation: %s", userRes))
		users := [2]string{"kube:admin", "system:masters"}
		Expect(users).To(ContainElement(result["metadata"].(map[string]interface{})["name"].(string)))
	})
})
