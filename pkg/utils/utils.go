package utils

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	configv1 "github.com/openshift/api/config/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	uberzap "go.uber.org/zap"
	uberzapcore "go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

func GetClusterClaim(ctx context.Context,
	k8sClient client.Client,
	name string,
) (*clustersv1alpha1.ClusterClaim, error) {
	clusterClaim := &clustersv1alpha1.ClusterClaim{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, clusterClaim)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return clusterClaim, nil
}

func PrintRuntimeInfo() {
	log := logger.DefaultZapLogger()
	log.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Infof("Go Version: %s", runtime.Version())
	log.Infof("Git Commit: %s", os.Getenv("GIT_COMMIT"))
}

func CtrlZapOptions() zap.Options {
	encoderConfig := uberzap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = uberzapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = uberzapcore.CapitalColorLevelEncoder
	opts := zap.Options{
		Encoder: uberzapcore.NewConsoleEncoder(encoderConfig),
		// for development
		// ZapOpts: []uberzap.Option{
		// 	uberzap.AddCaller(),
		// },
	}
	return opts
}

// Validate return true if the file exists and the content is not empty
func Validate(filePath string) (string, bool) {
	if len(filePath) == 0 {
		return "", false
	}
	content, err := os.ReadFile(filePath) // #nosec G304
	if err != nil {
		log.Infof("failed to read file %s - %v", filePath, err)
		return "", false
	}
	trimmedContent := strings.TrimSpace(string(content))
	return trimmedContent, len(trimmedContent) > 0
}

// GetDefaultNamespace returns default installation namespace
func GetDefaultNamespace() string {
	defaultNamespace, _ := os.LookupEnv("POD_NAMESPACE")
	if defaultNamespace == "" {
		defaultNamespace = constants.GHDefaultNamespace
	}
	return defaultNamespace
}

func ListMCH(ctx context.Context, k8sClient client.Client) (*mchv1.MultiClusterHub, error) {
	mch := &mchv1.MultiClusterHubList{}
	err := k8sClient.List(ctx, mch)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(mch.Items) == 0 {
		return nil, err
	}

	return &mch.Items[0], nil
}

func RestartPod(ctx context.Context, kubeClient kubernetes.Interface, podNamespace, deploymentName string) error {
	labelSelector := fmt.Sprintf("name=%s", deploymentName)
	poList, err := kubeClient.CoreV1().Pods(podNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	for _, po := range poList.Items {
		err := kubeClient.CoreV1().Pods(podNamespace).Delete(ctx, po.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func IsBackupEnabled(ctx context.Context, client client.Client) (bool, error) {
	mch, err := ListMCH(ctx, client)
	if err != nil {
		return false, err
	}
	if mch == nil {
		return false, nil
	}
	if mch.Spec.Overrides == nil {
		return false, nil
	}
	for _, c := range mch.Spec.Overrides.Components {
		if c.Name == "cluster-backup" && c.Enabled {
			return true, nil
		}
	}
	return false, nil
}

func GetClusterIdFromClusterVersion(c client.Client, ctx context.Context) (string, error) {
	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(clusterVersion), clusterVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get the ClusterVersion(version): %w", err)
	}

	clusterID := string(clusterVersion.Spec.ClusterID)
	if clusterID == "" {
		return "", fmt.Errorf("the clusterId from ClusterVersion must not be empty")
	}
	return clusterID, nil
}

// GetTopicACL creates a KafkaUserSpecAuthorizationAclsElem for a given topic name
// with the specified operations. The returned ACL element has a wildcard host
// and specifies the resource type as Topic.
func GetTopicACL(topicName string,
	operations []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem,
) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	patternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	writeAcl := kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
			Name:        &topicName,
			PatternType: &patternType,
		},
		Operations: operations,
	}
	return writeAcl
}

func WriteTopicACL(topicName string) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	patternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	writeAcl := kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
			Name:        &topicName,
			PatternType: &patternType,
		},
		Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		},
	}
	return writeAcl
}

func ReadTopicACL(topicName string, prefixPattern bool) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	patternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	if prefixPattern {
		patternType = kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypePrefix
	}

	return kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
			Name:        &topicName,
			PatternType: &patternType,
		},
		Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	}
}

func ConsumeGroupReadACL() kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	consumerGroup := "*"
	consumerPatternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	consumerAcl := kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup,
			Name:        &consumerGroup,
			PatternType: &consumerPatternType,
		},
		Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	}
	return consumerAcl
}

// GenerateACLKey generates an ACL key based on the provided ACL element.
func GenerateACLKey(acl kafkav1beta2.KafkaUserSpecAuthorizationAclsElem) string {
	// Sort operations so "Read,Write" and "Write,Read" are treated the same
	ops := make([]string, len(acl.Operations))
	for i, v := range acl.Operations {
		ops[i] = string(v)
	}
	sort.Strings(ops)
	return fmt.Sprintf("%s|%s", *acl.Resource.Name, strings.Join(ops, ","))
}
