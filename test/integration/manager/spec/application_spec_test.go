package spec

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test ./test/integration/manager/spec -v -ginkgo.focus "application to database controller"
var _ = Describe("application to database controller", func() {
	const testSchema = "spec"
	const testTable = "applications"

	It("Synchronize application to database", func() {
		By("Create application app1 instance with OwnerReference")
		instance := &appv1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app1",
				Namespace: utils.GetDefaultNamespace(),
				Labels:    map[string]string{constants.GlobalHubGlobalResourceLabel: ""},
			},
			Spec: appv1beta1.ApplicationSpec{},
		}
		Expect(runtimeClient.Create(ctx, instance)).Should(Succeed())

		Eventually(func() error {
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
				syncedApp := &appv1beta1.Application{}
				if err := json.Unmarshal(payload, syncedApp); err != nil {
					return err
				}

				fmt.Printf("spec.applications: %s - %s \n", syncedApp.Namespace, syncedApp.Name)
				if syncedApp.GetNamespace() == instance.GetNamespace() &&
					syncedApp.GetName() == instance.GetName() {
					return nil
				}
			}
			return fmt.Errorf("not find app(%s) in database", instance.GetName())
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
