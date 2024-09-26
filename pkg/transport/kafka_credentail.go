package transport

import "sigs.k8s.io/kustomize/kyaml/yaml"

// KafkaConnCredential is used to connect the transporter instance. The field is persisted to secret
// need to be encode with base64.StdEncoding.EncodeToString
type KafkaConnCredential struct {
	BootstrapServer  string `yaml:"bootstrap.server"`
	StatusTopic      string `yaml:"topic.status,omitempty"`
	SpecTopic        string `yaml:"topic.spec,omitempty"`
	ClusterID        string `yaml:"cluster.id,omitempty"`
	CACert           string `yaml:"ca.crt,omitempty"`
	ClientCert       string `yaml:"client.crt,omitempty"`
	ClientKey        string `yaml:"client.key,omitempty"`
	CASecretName     string `yaml:"ca.secret,omitempty"`
	ClientSecretName string `yaml:"client.secret,omitempty"`
}

// YamlMarshal marshal the connection credential object, rawCert specifies whether to keep the cert in the data directly
func (k *KafkaConnCredential) YamlMarshal(rawCert bool) ([]byte, error) {
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
func (k *KafkaConnCredential) DeepCopy() *KafkaConnCredential {
	return &KafkaConnCredential{
		BootstrapServer:  k.BootstrapServer,
		StatusTopic:      k.StatusTopic,
		SpecTopic:        k.SpecTopic,
		ClusterID:        k.ClusterID,
		CACert:           k.CACert,
		ClientCert:       k.ClientCert,
		ClientKey:        k.ClientKey,
		CASecretName:     k.CASecretName,
		ClientSecretName: k.ClientSecretName,
	}
}

func (k *KafkaConnCredential) GetCACert() string {
	return k.CACert
}

func (k *KafkaConnCredential) SetCACert(cert string) {
	k.CACert = cert
}

func (k *KafkaConnCredential) GetClientCert() string {
	return k.ClientCert
}

func (k *KafkaConnCredential) SetClientCert(cert string) {
	k.ClientCert = cert
}

func (k *KafkaConnCredential) GetClientKey() string {
	return k.ClientKey
}

func (k *KafkaConnCredential) SetClientKey(key string) {
	k.ClientKey = key
}

func (k *KafkaConnCredential) GetCASecretName() string {
	return k.CASecretName
}

func (k *KafkaConnCredential) GetClientSecretName() string {
	return k.ClientSecretName
}
