package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
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

	version := eventversion.NewVersion()

	BeforeEach(func() {
		// truncate table
		db := database.GetSqlDb()
		sql := fmt.Sprintf(`TRUNCATE TABLE %s.%s`, database.SecuritySchema, database.SecurityAlertCountsTable)
		_, err := db.Query(sql)
		Expect(err).To(Succeed())
	})

	It("Should be able to sync security alert counts event from one central instance in hub", func() {
		By("Create event")
		data := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source1,
		}
		version.Incr()
		event := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *event)
		Expect(err).To(Succeed())
		version.Next()

		By("Check the table")
		db := database.GetSqlDb()
		Expect(db).ToNot(BeNil())
		sql := fmt.Sprintf(
			`
				SELECT
					low,
					medium,
					high,
					critical,
					detail_url,
					source
				FROM
					%s.%s
				WHERE
					hub_name = $1
			`,
			database.SecuritySchema, database.SecurityAlertCountsTable,
		)
		check := func(g Gomega) {
			var (
				low       int
				medium    int
				high      int
				critical  int
				detailURL string
				source    string
			)
			row := db.QueryRow(sql, leafHubName)
			err := row.Scan(&low, &medium, &high, &critical, &detailURL, &source)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(low).To(Equal(1))
			g.Expect(medium).To(Equal(2))
			g.Expect(high).To(Equal(3))
			g.Expect(critical).To(Equal(4))
			g.Expect(detailURL).To(Equal(detailURL))
			g.Expect(source).To(Equal(source1))
		}
		Eventually(check, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("Should be able to sync security alert counts event from multiple central instances in hub", func() {
		By("Create event")
		dataEvent1 := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source1,
		}
		version.Incr()
		event1 := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, dataEvent1)

		By("Sync event1 with transport")
		err := producer.SendEvent(ctx, *event1)
		Expect(err).To(Succeed())
		version.Next()

		dataEvent2 := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source2,
		}
		version.Incr()
		event2 := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, dataEvent2)

		By("Sync event2 with transport")
		err = producer.SendEvent(ctx, *event2)
		Expect(err).To(Succeed())
		version.Next()

		By("Check the table")
		db := database.GetSqlDb()
		Expect(db).ToNot(BeNil())
		sql := fmt.Sprintf(
			`
				SELECT
					low,
					medium,
					high,
					critical,
					detail_url,
					source
				FROM
					%s.%s
				WHERE
					hub_name = $1 AND source = $2
			`,
			database.SecuritySchema, database.SecurityAlertCountsTable,
		)
		check := func(g Gomega) {
			var (
				low       int
				medium    int
				high      int
				critical  int
				detailURL string
				source    string
			)

			// verify first event added
			row := db.QueryRow(sql, leafHubName, source1)
			err = row.Scan(&low, &medium, &high, &critical, &detailURL, &source)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(low).To(Equal(1))
			g.Expect(medium).To(Equal(2))
			g.Expect(high).To(Equal(3))
			g.Expect(critical).To(Equal(4))
			g.Expect(detailURL).To(Equal(detailURL))
			g.Expect(source).To(Equal(source1))

			// verify second event added
			row = db.QueryRow(sql, leafHubName, source2)
			err = row.Scan(&low, &medium, &high, &critical, &detailURL, &source)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(low).To(Equal(1))
			g.Expect(medium).To(Equal(2))
			g.Expect(high).To(Equal(3))
			g.Expect(critical).To(Equal(4))
			g.Expect(detailURL).To(Equal(detailURL))
			g.Expect(source).To(Equal(source2))
		}
		Eventually(check, 30*time.Second, 100*time.Millisecond).Should(Succeed())
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

		_, err = db.Query(sql, leafHubName, DetailURL, source2)
		Expect(err).To(Succeed())

		By("Create event")
		dataEvent1 := &wiremodels.SecurityAlertCounts{
			Low:       4,
			Medium:    4,
			High:      4,
			Critical:  4,
			DetailURL: DetailURL,
			Source:    source1,
		}
		version.Incr()
		event1 := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, dataEvent1)

		By("Sync events with transport")
		err = producer.SendEvent(ctx, *event1)
		Expect(err).To(Succeed())
		version.Next()

		By("Check the table")
		sql = fmt.Sprintf(
			`
				SELECT
					low,
					medium,
					high,
					critical,
					detail_url,
					source
				FROM
					%s.%s
				WHERE
					hub_name = $1 AND source = $2
			`,
			database.SecuritySchema, database.SecurityAlertCountsTable,
		)
		check := func(g Gomega) {
			var (
				low       int
				medium    int
				high      int
				critical  int
				detailURL string
				source    string
			)

			// verify first event changed
			row := db.QueryRow(sql, leafHubName, source1)
			err := row.Scan(&low, &medium, &high, &critical, &detailURL, &source)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(low).To(Equal(4))
			g.Expect(medium).To(Equal(4))
			g.Expect(high).To(Equal(4))
			g.Expect(critical).To(Equal(4))
			g.Expect(detailURL).To(Equal(detailURL))
			g.Expect(source).To(Equal(source1))

			// verify second event not changed
			row = db.QueryRow(sql, leafHubName, source2)
			err = row.Scan(&low, &medium, &high, &critical, &detailURL, &source)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(low).To(Equal(1))
			g.Expect(medium).To(Equal(2))
			g.Expect(high).To(Equal(3))
			g.Expect(critical).To(Equal(4))
			g.Expect(detailURL).To(Equal(detailURL))
			g.Expect(source).To(Equal(source2))
		}
		Eventually(check, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})
})
