package status

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

var _ = Describe("TransportOffsetPersistence", Ordered, func() {
	var (
		committer  *conflator.ConflationCommitter
		testCtx    context.Context
		testCancel context.CancelFunc
		topic1     = "test-topic-1"
		topic2     = "test-topic-2"
	)

	BeforeEach(func() {
		// Create a new context for each test to ensure proper cleanup
		testCtx, testCancel = context.WithCancel(ctx)
	})

	AfterEach(func() {
		// Cancel the test context to stop any running committers
		if testCancel != nil {
			testCancel()
		}
		// Wait a moment for goroutines to stop
		time.Sleep(500 * time.Millisecond)
	})

	BeforeAll(func() {
		// Clean up any existing transport records
		db := database.GetGorm()
		Expect(db.Exec("DELETE FROM status.transport").Error).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		// Clean up test data
		db := database.GetGorm()
		Expect(db.Exec("DELETE FROM status.transport").Error).NotTo(HaveOccurred())
	})

	Context("When upgrading from old format to new format", func() {
		It("should migrate old transport records to new format", func() {
			db := database.GetGorm()

			// Insert old format records (topic name only, without @partition)
			oldRecords := []models.Transport{
				{
					Name:    "old-topic-1",
					Payload: []byte(`{"ownerIdentity":"test-owner","partition":0,"offset":100}`),
				},
				{
					Name:    "old-topic-2",
					Payload: []byte(`{"ownerIdentity":"test-owner","partition":1,"offset":200}`),
				},
				{
					Name:    "old-topic-3",
					Payload: []byte(`{"ownerIdentity":"test-owner","partition":2,"offset":300}`),
				},
			}

			for _, record := range oldRecords {
				err := db.Create(&record).Error
				Expect(err).NotTo(HaveOccurred())
			}

			// Create a dummy metadata function
			metadataFunc := func() []conflator.ConflationMetadata {
				return []conflator.ConflationMetadata{}
			}

			// Create committer and trigger migration
			committer = conflator.NewKafkaConflationCommitter(metadataFunc)
			err := committer.Start(testCtx)
			Expect(err).NotTo(HaveOccurred())

			// Wait a moment for migration to complete
			time.Sleep(1 * time.Second)

			// Verify old records are gone
			var oldCount int64
			err = db.Model(&models.Transport{}).Where("name NOT LIKE ?", "%@%").Count(&oldCount).Error
			Expect(err).NotTo(HaveOccurred())
			Expect(oldCount).To(Equal(int64(0)), "old format records should be deleted")

			// Verify new records exist with correct format
			var newRecords []models.Transport
			err = db.Find(&newRecords).Error
			Expect(err).NotTo(HaveOccurred())
			Expect(newRecords).To(HaveLen(3))

			// Verify each new record has topic@partition format
			expectedNames := map[string]bool{
				"old-topic-1@0": false,
				"old-topic-2@1": false,
				"old-topic-3@2": false,
			}

			for _, record := range newRecords {
				_, exists := expectedNames[record.Name]
				Expect(exists).To(BeTrue(), "Record name should be in expected format: %s", record.Name)
				expectedNames[record.Name] = true

				// Verify payload is preserved
				var eventPos transport.EventPosition
				err := json.Unmarshal(record.Payload, &eventPos)
				Expect(err).NotTo(HaveOccurred())
				Expect(eventPos.OwnerIdentity).To(Equal("test-owner"))
			}

			// Verify all expected records were found
			for name, found := range expectedNames {
				Expect(found).To(BeTrue(), "Expected record not found: %s", name)
			}

			// Clean up for next tests
			Expect(db.Exec("DELETE FROM status.transport").Error).NotTo(HaveOccurred())
		})
	})

	Context("When committing offsets for multiple partitions of the same topic", func() {
		It("should successfully store offsets without duplicate key errors", func() {
			// Create a metadata function that returns multiple partitions for the same topic
			metadataFunc := func() []conflator.ConflationMetadata {
				return []conflator.ConflationMetadata{
					&testMetadata{
						topic:     topic1,
						partition: 0,
						offset:    100,
						processed: true,
					},
					&testMetadata{
						topic:     topic1,
						partition: 1,
						offset:    200,
						processed: true,
					},
					&testMetadata{
						topic:     topic1,
						partition: 2,
						offset:    300,
						processed: true,
					},
					&testMetadata{
						topic:     topic2,
						partition: 0,
						offset:    150,
						processed: true,
					},
				}
			}

			// Create committer with test metadata
			committer = conflator.NewKafkaConflationCommitter(metadataFunc)

			// Trigger commit - this should not fail with duplicate key error
			err := committer.Start(testCtx)
			Expect(err).NotTo(HaveOccurred())

			// Wait for the commit to happen (committer runs every 10 seconds)
			time.Sleep(12 * time.Second)

			// Verify the records in database
			db := database.GetGorm()
			var positions []models.Transport
			err = db.Find(&positions).Error
			Expect(err).NotTo(HaveOccurred())

			// Should have 4 records (3 partitions for topic1 + 1 partition for topic2)
			Expect(positions).To(HaveLen(4))

			// Verify each record has the correct format: topic@partition
			nameSet := make(map[string]bool)
			for _, pos := range positions {
				nameSet[pos.Name] = true

				// Verify payload contains the correct partition and offset
				var kafkaPos transport.EventPosition
				err := json.Unmarshal(pos.Payload, &kafkaPos)
				Expect(err).NotTo(HaveOccurred())

				// Verify ownerIdentity is set
				Expect(kafkaPos.OwnerIdentity).To(Equal(config.GetKafkaOwnerIdentity()))
			}

			// Verify all expected keys exist
			Expect(nameSet).To(HaveKey("test-topic-1@0"))
			Expect(nameSet).To(HaveKey("test-topic-1@1"))
			Expect(nameSet).To(HaveKey("test-topic-1@2"))
			Expect(nameSet).To(HaveKey("test-topic-2@0"))
		})

		It("should correctly store topic and partition in separate fields", func() {
			// Verify that the topic name can be extracted from the Name field
			db := database.GetGorm()
			var positions []models.Transport
			err := db.Find(&positions).Error
			Expect(err).NotTo(HaveOccurred())

			for _, pos := range positions {
				// Name should be in format "topic@partition"
				parts := strings.Split(pos.Name, "@")
				Expect(parts).To(HaveLen(2), "Name should be in format topic@partition")

				// Extract the actual topic and partition from payload
				var kafkaPos transport.EventPosition
				err := json.Unmarshal(pos.Payload, &kafkaPos)
				Expect(err).NotTo(HaveOccurred())

				// Verify the topic in the name matches
				topic := parts[0]
				Expect(topic).To(Or(Equal(topic1), Equal(topic2)))

				// Verify partition is correctly stored in payload
				Expect(kafkaPos.Partition).To(BeNumerically(">=", 0))
				Expect(kafkaPos.Partition).To(BeNumerically("<", 3))
			}
		})

		It("should handle updates to existing offsets", func() {
			// Update metadata with new offsets
			metadataFunc := func() []conflator.ConflationMetadata {
				return []conflator.ConflationMetadata{
					&testMetadata{
						topic:     topic1,
						partition: 0,
						offset:    150, // Updated offset
						processed: true,
					},
					&testMetadata{
						topic:     topic1,
						partition: 1,
						offset:    250, // Updated offset
						processed: true,
					},
				}
			}

			// Create new committer with updated metadata
			committer = conflator.NewKafkaConflationCommitter(metadataFunc)
			err := committer.Start(testCtx)
			Expect(err).NotTo(HaveOccurred())

			// Wait for commit
			time.Sleep(12 * time.Second)

			// Verify the records were updated
			db := database.GetGorm()
			var position models.Transport
			err = db.Where("name = ?", "test-topic-1@0").First(&position).Error
			Expect(err).NotTo(HaveOccurred())

			var kafkaPos transport.EventPosition
			err = json.Unmarshal(position.Payload, &kafkaPos)
			Expect(err).NotTo(HaveOccurred())
			Expect(kafkaPos.Offset).To(Equal(int64(150)))
		})
	})
})

// testMetadata implements conflator.ConflationMetadata for testing
type testMetadata struct {
	topic     string
	partition int32
	offset    int64
	processed bool
}

func (m *testMetadata) TransportPosition() *transport.EventPosition {
	return &transport.EventPosition{
		OwnerIdentity: config.GetKafkaOwnerIdentity(),
		Topic:         m.topic,
		Partition:     m.partition,
		Offset:        m.offset,
	}
}

func (m *testMetadata) Processed() bool {
	return m.processed
}

func (m *testMetadata) MarkAsProcessed() {
	m.processed = true
}

func (m *testMetadata) MarkAsUnprocessed() {
	m.processed = false
}

func (m *testMetadata) Version() *version.Version {
	return version.NewVersion()
}

func (m *testMetadata) DependencyVersion() *version.Version {
	return version.NewVersion()
}

func (m *testMetadata) EventType() string {
	return "test-event"
}
