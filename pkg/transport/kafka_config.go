package transport

import "sigs.k8s.io/yaml"

// KafkaConfig is used to connect the transporter instance.
// This struct can be marshalled into a single Secret entry like "kafka.yaml".
type KafkaConfig struct {
	BootstrapServer  string `json:"bootstrapServer,omitempty" yaml:"bootstrapServer,omitempty"`
	StatusTopic      string `json:"statusTopic,omitempty" yaml:"statusTopic,omitempty"`
	SpecTopic        string `json:"specTopic,omitempty" yaml:"specTopic,omitempty"`
	ClusterID        string `json:"clusterID,omitempty" yaml:"clusterID,omitempty"`
	ConsumerGroupID  string `json:"consumerGroupID,omitempty" yaml:"consumerGroupID,omitempty"`
	CACert           string `json:"caCert,omitempty" yaml:"caCert,omitempty"`
	ClientCert       string `json:"clientCert,omitempty" yaml:"clientCert,omitempty"`
	ClientKey        string `json:"clientKey,omitempty" yaml:"clientKey,omitempty"`
	CASecretName     string `json:"caSecretName,omitempty" yaml:"caSecretName,omitempty"`
	ClientSecretName string `json:"clientSecretName,omitempty" yaml:"clientSecretName,omitempty"`
}

// YamlMarshal marshal the connection credential object, rawCert specifies whether to keep the cert in the data directly
func (k *KafkaConfig) YamlMarshal(rawCert bool) ([]byte, error) {
	copy := k.DeepCopy()
	if rawCert {
		copy.CASecretName = ""
		copy.ClientSecretName = ""
	} else {
		copy.CACert = ""
		copy.ClientCert = ""
		copy.ClientKey = ""
	}
	bytes, err := yaml.Marshal(copy)
	return bytes, err
}

// DeepCopy creates a deep copy of KafkaConnCredential
func (k *KafkaConfig) DeepCopy() *KafkaConfig {
	return &KafkaConfig{
		BootstrapServer:  k.BootstrapServer,
		StatusTopic:      k.StatusTopic,
		SpecTopic:        k.SpecTopic,
		ClusterID:        k.ClusterID,
		ConsumerGroupID:  k.ConsumerGroupID,
		CACert:           k.CACert,
		ClientCert:       k.ClientCert,
		ClientKey:        k.ClientKey,
		CASecretName:     k.CASecretName,
		ClientSecretName: k.ClientSecretName,
	}
}

func (k *KafkaConfig) GetCACert() string {
	return k.CACert
}

func (k *KafkaConfig) SetCACert(cert string) {
	k.CACert = cert
}

func (k *KafkaConfig) GetClientCert() string {
	return k.ClientCert
}

func (k *KafkaConfig) SetClientCert(cert string) {
	k.ClientCert = cert
}

func (k *KafkaConfig) GetClientKey() string {
	return k.ClientKey
}

func (k *KafkaConfig) SetClientKey(key string) {
	k.ClientKey = key
}

func (k *KafkaConfig) GetCASecretName() string {
	return k.CASecretName
}

func (k *KafkaConfig) GetClientSecretName() string {
	return k.ClientSecretName
}
