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

var _ = Describe("SecurityAlertCountsHandler", Ordered, func() {
	It("Should be able to sync security alert counts event", func() {
		By("Create event")
		const leafHubName = "hub1"
		version := eventversion.NewVersion()
		version.Incr()
		data := &wiremodels.SecurityAlertCounts{
			Low:       1,
			Medium:    2,
			High:      3,
			Critical:  4,
			DetailURL: "https://hub1/violations",
		}
		event := ToCloudEvent(leafHubName, string(enum.SecurityAlertCountsType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *event)
		Expect(err).To(Succeed())

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
					detail_url
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
			)
			row := db.QueryRow(sql, leafHubName)
			err := row.Scan(&low, &medium, &high, &critical, &detailURL)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(low).To(Equal(1))
			g.Expect(medium).To(Equal(2))
			g.Expect(high).To(Equal(3))
			g.Expect(critical).To(Equal(4))
			g.Expect(detailURL).To(Equal("https://hub1/violations"))
		}
		Eventually(check, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})
})
