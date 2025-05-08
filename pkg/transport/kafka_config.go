package transport

import "sigs.k8s.io/kustomize/kyaml/yaml"

// KafkaConfig is used to connect the transporter instance. The field is persisted to secret
// need to be encode with base64.StdEncoding.EncodeToString
type KafkaConfig struct {
	BootstrapServer   string `yaml:"bootstrap.server"`
	StatusTopic       string `yaml:"topic.status,omitempty"`
	SpecTopic         string `yaml:"topic.spec,omitempty"`
	ClusterID         string `yaml:"cluster.id,omitempty"`
	CACert            string `yaml:"ca.crt,omitempty"`
	ClientCert        string `yaml:"client.crt,omitempty"`
	ClientKey         string `yaml:"client.key,omitempty"`
	CASecretName      string `yaml:"ca.secret,omitempty"`
	ClientSecretName  string `yaml:"client.secret,omitempty"`
	IsNewKafkaCluster bool   `yaml:"isNewKafkaCluster,omitempty"`
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
		BootstrapServer:   k.BootstrapServer,
		StatusTopic:       k.StatusTopic,
		SpecTopic:         k.SpecTopic,
		ClusterID:         k.ClusterID,
		CACert:            k.CACert,
		ClientCert:        k.ClientCert,
		ClientKey:         k.ClientKey,
		CASecretName:      k.CASecretName,
		ClientSecretName:  k.ClientSecretName,
		IsNewKafkaCluster: k.IsNewKafkaCluster,
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
