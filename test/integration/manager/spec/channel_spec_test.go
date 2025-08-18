package spec

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test ./test/integration/manager/spec -v -ginkgo.focus "channels to database controller"
var _ = Describe("channels to database controller", func() {
	const testSchema = "spec"
	const testTable = "channels"

	It("filter channel with MCH OwnerReferences", func() {
		By("Create channel ch1 instance with OwnerReference")
		filteredChannel := &channelv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ch1",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: channelv1.ChannelSpec{
				Type:     channelv1.ChannelTypeGit,
				Pathname: "test-path",
			},
		}
		Expect(controllerutil.SetControllerReference(multiclusterhub, filteredChannel,
			mgr.GetScheme())).NotTo(HaveOccurred())
		Expect(runtimeClient.Create(ctx, filteredChannel)).Should(Succeed())

		By("Create channel ch2 instance without OwnerReference")
		expectedChannel := &channelv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ch2",
				Namespace: utils.GetDefaultNamespace(),
				Labels:    map[string]string{constants.GlobalHubGlobalResourceLabel: ""},
			},
			Spec: channelv1.ChannelSpec{
				Type:     channelv1.ChannelTypeGit,
				Pathname: "test-path",
			},
		}
		Expect(runtimeClient.Create(ctx, expectedChannel)).Should(Succeed())

		Eventually(func() error {
			expectedChannelSynced := false

			rows, err := database.GetGorm().Raw(fmt.Sprintf("SELECT payload FROM %s.%s", testSchema, testTable)).Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					log.Printf("failed to close rows: %v", err)
				}
			}()
			for rows.Next() {

				var payload []byte
				err := rows.Scan(&payload)
				if err != nil {
					return err
				}
				syncedChannel := &channelv1.Channel{}
				if err := json.Unmarshal(payload, syncedChannel); err != nil {
					return err
				}

				fmt.Printf("spec.channels: %s - %s \n", syncedChannel.Namespace, syncedChannel.Name)
				if syncedChannel.GetNamespace() == expectedChannel.GetNamespace() &&
					syncedChannel.GetName() == expectedChannel.GetName() {
					expectedChannelSynced = true
				}
				if syncedChannel.GetNamespace() == filteredChannel.GetNamespace() &&
					syncedChannel.GetName() == filteredChannel.GetName() {
					return fmt.Errorf("channel(%s) with OwnerReference(MCH) should't be synchronized to database",
						filteredChannel.GetName())
				}
			}
			if expectedChannelSynced {
				return nil
			}
			return fmt.Errorf("not find channel(%s) in database", expectedChannel.GetName())
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
