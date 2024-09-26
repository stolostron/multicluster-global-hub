package status

import (
	"context"
	"fmt"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	wiremodels "github.com/stolostron/multicluster-global-hub/pkg/wire/models"
)

// go test ./test/integration/manager/status -v -ginkgo.focus "SecurityAlertCountsHandler"
var _ = Describe("SecurityAlertCountsHandler", Ordered, func() {
	const (
		leafHubName = "hub1"
		source1     = "rhacs-operator/stackrox-central-services"
		source2     = "other-namespace/other-name"
		DetailURL   = "https://hub1/violations"
	)

	var (
		version        = eventversion.NewVersion()
		statusTopicCtx context.Context
	)
	BeforeEach(func() {
		db := database.GetSqlDb()
		sql := fmt.Sprintf(`TRUNCATE TABLE %s.%s`, database.SecuritySchema, database.SecurityAlertCountsTable)
		_, err := db.Query(sql)
		Expect(err).To(Succeed())

		statusTopicCtx = cecontext.WithTopic(ctx, "event")
	})

	It("Should be able to sync security alert counts event from one central instance in hub", func() {
		By("Create event")
		expectedAlert := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source1,
		}
		version.Incr()
		event := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, expectedAlert)

		By("Sync event with transport")
		err := producer.SendEvent(statusTopicCtx, *event)
		Expect(err).To(Succeed())
		version.Next()

		By("Check the table")
		Eventually(func() error {
			db := database.GetGorm()
			gotAlert := &models.SecurityAlertCounts{}
			err = db.First(&gotAlert).Error
			if err != nil {
				return err
			}
			if !assertAlert(expectedAlert, gotAlert) {
				return fmt.Errorf("want %v, but got %v", expectedAlert, gotAlert)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("Should be able to sync security alert counts event from multiple central instances in hub", func() {
		By("Create event")
		expectAlert1 := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source1,
		}
		version.Incr()
		event1 := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, expectAlert1)

		By("Sync event1 with transport")
		err := producer.SendEvent(statusTopicCtx, *event1)
		Expect(err).To(Succeed())
		version.Next()

		// Since the security syncer(agent) doesn't use the bundle to sync message, the bundle contain several alert
		// count. then it might cause race condition when sending a lot of events at the same time.
		// Solution: we can migrate the security syncer to use the generic syncer implementation.
		By("Check the table")
		Eventually(func() error {
			db := database.GetGorm()
			// check source 1
			gotAlert1 := &models.SecurityAlertCounts{}
			err = db.Where(&models.SecurityAlertCounts{HubName: leafHubName, Source: source1}).First(&gotAlert1).Error
			if err != nil {
				return err
			}
			if !assertAlert(expectAlert1, gotAlert1) {
				return fmt.Errorf("want %v, but got %v", expectAlert1, gotAlert1)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		expectAlert2 := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source2,
		}
		version.Incr()
		event2 := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, expectAlert2)

		By("Sync event2 with transport")
		err = producer.SendEvent(statusTopicCtx, *event2)
		Expect(err).To(Succeed())
		version.Next()

		By("Check the table")
		Eventually(func() error {
			db := database.GetGorm()
			// check source 2
			gotAlert2 := &models.SecurityAlertCounts{}
			err = db.Where(&models.SecurityAlertCounts{HubName: leafHubName, Source: source2}).First(&gotAlert2).Error
			if err != nil {
				return err
			}
			if !assertAlert(expectAlert2, gotAlert2) {
				return fmt.Errorf("want %v, but got %v", expectAlert1, gotAlert2)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("Should be able to sync security alert counts event from multiple central instances in hub by updating only the necessary record", func() {
		By("Add records to the table")
		db := database.GetSqlDb()
		Expect(db).ToNot(BeNil())

		sql := fmt.Sprintf(
			`
				INSERT INTO %s.%s (hub_name, low, medium, high, critical, detail_url, source)
				VALUES ($1, 1, 2, 3, 4, $2, $3)
			`,
			database.SecuritySchema, database.SecurityAlertCountsTable,
		)

		_, err := db.Query(sql, leafHubName, DetailURL, source1)
		Expect(err).To(Succeed())

		expectAlert2 := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source2,
		}
		_, err = db.Query(sql, leafHubName, expectAlert2.DetailURL, expectAlert2.Source)
		Expect(err).To(Succeed())

		By("Create event")
		expectAlert1 := &wiremodels.SecurityAlertCounts{
			Low:       4,
			Medium:    4,
			High:      4,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source1,
		}
		version.Incr()
		event1 := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, expectAlert1)

		By("Sync events with transport")
		err = producer.SendEvent(statusTopicCtx, *event1)
		Expect(err).To(Succeed())
		version.Next()

		By("Check the table")
		Eventually(func() error {
			db := database.GetGorm()

			// check source1 has been modified
			gotAlert1 := &models.SecurityAlertCounts{}
			err = db.Where(&models.SecurityAlertCounts{HubName: leafHubName, Source: source1}).First(&gotAlert1).Error
			if err != nil {
				return err
			}
			if !assertAlert(expectAlert1, gotAlert1) {
				return fmt.Errorf("want %v, but got %v", expectAlert1, gotAlert1)
			}

			// check sources hasn't been modified
			gotAlert2 := &models.SecurityAlertCounts{}
			err = db.Where(&models.SecurityAlertCounts{HubName: leafHubName, Source: source2}).First(&gotAlert2).Error
			if err != nil {
				return err
			}
			if !assertAlert(expectAlert2, gotAlert2) {
				return fmt.Errorf("want %v, but got %v", expectAlert1, gotAlert2)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})
})

func assertAlert(expected *wiremodels.SecurityAlertCounts, got *models.SecurityAlertCounts) bool {
	return expected.Low == got.Low &&
		expected.Medium == got.Medium &&
		expected.High == got.High &&
		expected.Critical == got.Critical &&
		expected.DetailURL == got.DetailURL &&
		expected.Source == got.Source
}
