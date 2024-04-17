package config

import (
	"context"
	"strconv"

	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var controllerConfigMap *corev1.ConfigMap

func LoadControllerConfig(ctx context.Context, kubeClient *kubernetes.Clientset) error {
	config, err := kubeClient.CoreV1().ConfigMaps(utils.GetDefaultNamespace()).Get(
		ctx, operatorconstants.ControllerConfig, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	controllerConfigMap = config
	return nil
}

func SetControllerConfig(controllerConfigMap *corev1.ConfigMap) {
	controllerConfigMap = controllerConfigMap
}

func GetElectionConfig() (*commonobjects.LeaderElectionConfig, error) {
	config := &commonobjects.LeaderElectionConfig{
		LeaseDuration: 137,
		RenewDeadline: 107,
		RetryPeriod:   26,
	}
	if controllerConfigMap == nil {
		return config, nil
	}
	val, leaseDurationExist := controllerConfigMap.Data["leaseDuration"]
	if leaseDurationExist {
		leaseDurationSec, err := strconv.Atoi(val)
		if err != nil {
			return nil, err
		}
		config.LeaseDuration = leaseDurationSec
	}

	val, renewDeadlineExist := controllerConfigMap.Data["renewDeadline"]
	if renewDeadlineExist {
		renewDeadlineSec, err := strconv.Atoi(val)
		if err != nil {
			return nil, err
		}
		config.RenewDeadline = renewDeadlineSec
	}

	val, retryPeriodExist := controllerConfigMap.Data["retryPeriod"]
	if retryPeriodExist {
		retryPeriodSec, err := strconv.Atoi(val)
		if err != nil {
			return nil, err
		}
		config.RetryPeriod = retryPeriodSec
	}

	return config, nil
}

// getAgentRestConfig return the agent qps and burst, if not set, use default QPS and Burst
func GetAgentRestConfig() (float32, int) {
	if controllerConfigMap == nil {
		return constants.DefaultAgentQPS, constants.DefaultAgentBurst
	}

	agentQPS := constants.DefaultAgentQPS
	_, agentQPSExist := controllerConfigMap.Data["agentQPS"]
	if agentQPSExist {
		agentQPSInt, err := strconv.Atoi(controllerConfigMap.Data["agentQPS"])
		if err == nil {
			agentQPS = float32(agentQPSInt)
		}
	}

	agentBurst := constants.DefaultAgentBurst
	_, agentBurstExist := controllerConfigMap.Data["agentBurst"]
	if agentBurstExist {
		agentBurstAtoi, err := strconv.Atoi(controllerConfigMap.Data["agentBurst"])
		if err == nil {
			agentBurst = agentBurstAtoi
		}
	}
	return agentQPS, agentBurst
}
