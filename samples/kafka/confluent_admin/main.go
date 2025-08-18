package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

// create topic with admin
func main() {
	if len(os.Args) < 2 {
		panic("Please provide at least one topic command-line argument.")
	}
	topic := os.Args[1]
	fmt.Println("topic", topic)

	kafkaConfigMap, err := config.GetConfluentConfigMapByCurrentUser(
		"multicluster-global-hub", "kafka", "admin-kafka-user")
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
	if err := grantWrite(admin, "managed-hub", topic); err != nil {
		panic(err)
	}
	if err := grantRead(admin, "managed-hub", topic); err != nil {
		panic(err)
	}

	admin.Close()

	fmt.Println("Done")
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
