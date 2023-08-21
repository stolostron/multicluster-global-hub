package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("leaf hubs heartbeats", Ordered, func() {
	const (
		testSchema  = database.StatusSchema
		testTable   = database.LeafHubHeartbeatsTableName
		leafHubName = "hub1"
	)

	BeforeAll(func() {
		By("Check whether the tables are created")
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx, "SELECT * FROM pg_tables")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				columnValues, _ := rows.Values()
				schema := columnValues[0]
				table := columnValues[1]
				if schema == testSchema && table == testTable {
					return nil
				}
			}
			return fmt.Errorf("failed to create table %s.%s", testSchema, testTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync ControlInfo bundle", func() {
		version := status.NewBundleVersion()
		By("Create ControlInfo bundle")
		controlInfoBundle := &Bundle{
			LeafHubName:   leafHubName,
			BundleVersion: version,
			lock:          sync.Mutex{},
		}

		By("Create transport message")
		// increment the version
		controlInfoBundle.BundleVersion.Incr()
		payloadBytes, err := json.Marshal(controlInfoBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, constants.ControlInfoMsgKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			ID:      transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: controlInfoBundle.BundleVersion.String(),
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the heartbeats table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,last_timestamp FROM %s.%s", testSchema, testTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var hubName string
				var timestamp *time.Time
				if err := rows.Scan(&hubName, &timestamp); err != nil {
					return err
				}
				if hubName == controlInfoBundle.LeafHubName {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

// TODO: this bundle is from agent. we may need to combine the manager and agent bundle to common pkg
type Bundle struct {
	LeafHubName   string                `json:"leafHubName"`
	BundleVersion *status.BundleVersion `json:"bundleVersion"`
	lock          sync.Mutex
}
