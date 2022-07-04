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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/hub-of-hubs/test/pkg/utils"
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
		hohConfigGVR := utils.NewHoHConfigGVR()
		configList, err := dynamicClient.Resource(hohConfigGVR).Namespace("hoh-system").List(context.TODO(), metav1.ListOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(configList.Items) > 0).To(BeTrue())
	})

	It("non-k8s-api", func() {
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

		body, err := ioutil.ReadAll(resp.Body)
		Expect(err).ShouldNot(HaveOccurred())

		var result map[string]interface{}
		json.Unmarshal(body, &result)
		userRes, _ := json.MarshalIndent(result, "", "  ")
		klog.V(6).Info(fmt.Sprintf("The Test User Infomation: %s", userRes))
		users := [2]string{"kube:admin", "system:masters"}
		Expect(users).To(ContainElement(result["metadata"].(map[string]interface{})["name"].(string)))
	})
})
