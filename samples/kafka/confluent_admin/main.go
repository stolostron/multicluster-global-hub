package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

// create topic with admin
func main() {
	if len(os.Args) < 2 {
		panic("Please provide at least one topic command-line argument.")
	}
	topic := os.Args[1]
	fmt.Println("topic", topic)

	kafkaConfigMap, err := config.GetConfluentConfigMapByUser("multicluster-global-hub", "kafka", "admin-kafka-user")
	if err != nil {
		log.Fatalf("failed to get kafka config map: %v", err)
	}

	// Create a new AdminClient.
	// AdminClient can also be instantiated using an existing
	// Producer or Consumer instance, see NewAdminClientFromProducer and
	// NewAdminClientFromConsumer.
	admin, err := kafka.NewAdminClient(kafkaConfigMap)
	if err != nil {
		panic(fmt.Sprintf("Failed to create Admin client: %s\n", err))
	}

	// listTopic(admin)
	// err = describeACLs(admin, "managed-hub", topic)
	err = grantWrite(admin, "managed-hub", topic)
	err = grantRead(admin, "managed-hub", topic)
	if err != nil {
		panic(err)
	}

	admin.Close()

	fmt.Println("Done")
}

func listTopic(admin *kafka.AdminClient) {
	// List topics
	topics, err := admin.GetMetadata(nil, true, 5000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get metadata: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("List of topics:")
	for _, topic := range topics.Topics {
		fmt.Println(topic.Topic, len(topic.Partitions))
	}
}

func createTopic(admin *kafka.AdminClient, topic string) {
	// Create topics on cluster.
	// Set Admin options to wait for the operation to finish (or at most 60s)
	maxDur, err := time.ParseDuration("60s")
	if err != nil {
		panic("ParseDuration(60s)")
	}

	results, err := admin.CreateTopics(
		context.TODO(),
		// Multiple topics can be created simultaneously
		// by providing more TopicSpecification structs here.
		[]kafka.TopicSpecification{{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		}},
		// Admin options
		kafka.SetAdminOperationTimeout(maxDur))
	if err != nil {
		panic(fmt.Sprintf("Failed to create topic: %v\n", err))
	}

	// Print results
	for _, result := range results {
		fmt.Printf("%s\n", result.Error.String())
	}
}

func grantWrite(admin *kafka.AdminClient, clusterName, topicName string) error {
	// grant the managed hub user permission with admin client and CommonName in cert
	commonName := operatorconfig.GetKafkaUserName(clusterName)
	fmt.Println("KafkaUser: ", commonName)
	expectedACLBindings := []kafka.ACLBinding{
		{
			Type:                kafka.ResourceGroup,
			Name:                "*",
			ResourcePatternType: kafka.ResourcePatternTypeLiteral,
			Principal:           fmt.Sprintf("User:CN=%s", commonName),
			Host:                "*",
			Operation:           kafka.ACLOperationAll,
			PermissionType:      kafka.ACLPermissionTypeAllow,
		},
		{
			Type:                kafka.ResourceTopic,
			Name:                topicName,
			ResourcePatternType: kafka.ResourcePatternTypeLiteral,
			Principal:           fmt.Sprintf("User:CN=%s", commonName),
			Host:                "*",
			Operation:           kafka.ACLOperationWrite,
			PermissionType:      kafka.ACLPermissionTypeAllow,
		},
		{
			Type:                kafka.ResourceTopic,
			Name:                topicName,
			ResourcePatternType: kafka.ResourcePatternTypeLiteral,
			Principal:           fmt.Sprintf("User:CN=%s", commonName),
			Host:                "*",
			Operation:           kafka.ACLOperationDescribe,
			PermissionType:      kafka.ACLPermissionTypeAllow,
		},
	}

	aclBindings := []kafka.ACLBinding{}
	for _, acl := range expectedACLBindings {
		result, err := admin.DescribeACLs(context.TODO(), acl)
		if err != nil {
			return err
		}

		if hasACL(result, acl) {
			fmt.Printf("acl: %s -> %s/%s -> %s/%s \n", acl.Principal, acl.PermissionType.String(),
				acl.Operation.String(), acl.Type, acl.Name)
			continue
		}
		fmt.Printf("creating acl: %s -> %s/%s -> %s/%s \n", acl.Principal, acl.PermissionType.String(),
			acl.Operation.String(), acl.Type, acl.Name)
		aclBindings = append(aclBindings, acl)
	}

	if len(aclBindings) == 0 {
		return nil
	}

	results, err := admin.CreateACLs(context.TODO(), aclBindings)
	if err != nil {
		return err
	}

	for _, r := range results {
		fmt.Println("create result", r.Error)
		if r.Error.Code() != kafka.ErrNoError {
			return fmt.Errorf("CreateACLs failed, error code: %s, meesage: %s", r.Error.Code(), r.Error.String())
		}
	}

	return nil
}

func grantRead(admin *kafka.AdminClient, clusterName, topicName string) error {
	// grant the managed hub user permission with admin client and CommonName in cert
	commonName := operatorconfig.GetKafkaUserName(clusterName)
	fmt.Println("KafkaUser: ", commonName)
	expectedACLBindings := []kafka.ACLBinding{
		{
			Type:                kafka.ResourceGroup,
			Name:                "*",
			ResourcePatternType: kafka.ResourcePatternTypeLiteral,
			Principal:           fmt.Sprintf("User:CN=%s", commonName),
			Host:                "*",
			Operation:           kafka.ACLOperationAll,
			PermissionType:      kafka.ACLPermissionTypeAllow,
		},
		{
			Type:                kafka.ResourceTopic,
			Name:                topicName,
			ResourcePatternType: kafka.ResourcePatternTypeLiteral,
			Principal:           fmt.Sprintf("User:CN=%s", commonName),
			Host:                "*",
			Operation:           kafka.ACLOperationRead,
			PermissionType:      kafka.ACLPermissionTypeAllow,
		},
		{
			Type:                kafka.ResourceTopic,
			Name:                topicName,
			ResourcePatternType: kafka.ResourcePatternTypeLiteral,
			Principal:           fmt.Sprintf("User:CN=%s", commonName),
			Host:                "*",
			Operation:           kafka.ACLOperationDescribe,
			PermissionType:      kafka.ACLPermissionTypeAllow,
		},
	}

	aclBindings := []kafka.ACLBinding{}
	for _, acl := range expectedACLBindings {
		result, err := admin.DescribeACLs(context.TODO(), acl)
		if err != nil {
			return err
		}

		if hasACL(result, acl) {
			fmt.Printf("acl: %s -> %s/%s -> %s/%s \n", acl.Principal, acl.PermissionType.String(),
				acl.Operation.String(), acl.Type, acl.Name)
			continue
		}
		fmt.Printf("creating acl: %s -> %s/%s -> %s/%s \n", acl.Principal, acl.PermissionType.String(),
			acl.Operation.String(), acl.Type, acl.Name)
		aclBindings = append(aclBindings, acl)
	}

	if len(aclBindings) == 0 {
		return nil
	}

	results, err := admin.CreateACLs(context.TODO(), aclBindings)
	if err != nil {
		return err
	}

	for _, r := range results {
		fmt.Println("create result", r.Error)
		if r.Error.Code() != kafka.ErrNoError {
			return fmt.Errorf("CreateACLs failed, error code: %s, meesage: %s", r.Error.Code(), r.Error.String())
		}
	}

	return nil
}

func describeACLs(admin *kafka.AdminClient, userName, topicName string) error {
	// grant the managed hub user permission with admin client and CommonName in cert
	expectedAcl := kafka.ACLBinding{
		Type:                kafka.ResourceTopic,
		Name:                topicName,
		ResourcePatternType: kafka.ResourcePatternTypeLiteral,
		Principal:           fmt.Sprintf("User:CN=%s", userName),
		Operation:           kafka.ACLOperationWrite,
		PermissionType:      kafka.ACLPermissionTypeAllow,
	}
	existACLs, err := admin.DescribeACLs(context.TODO(), expectedAcl)
	if err != nil {
		return fmt.Errorf("failed the describe ACLs: %v", err)
	}

	fmt.Println("the acls:", existACLs.ACLBindings.Len())
	for _, acl := range existACLs.ACLBindings {
		fmt.Println("--------------------------")
		utils.PrettyPrint(acl)
	}
	return nil
}

func hasACL(acls *kafka.DescribeACLsResult, binding kafka.ACLBinding) bool {
	if acls.Error.Code() == kafka.ErrNoError {
		for _, a := range acls.ACLBindings {
			if a.Name == binding.Name && a.Type == binding.Type &&
				a.Operation.String() == binding.Operation.String() &&
				a.Principal == binding.Principal {
				return true
			}
		}
	}
	return false
}
