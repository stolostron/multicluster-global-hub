package transport

import "sigs.k8s.io/kustomize/kyaml/yaml"

type RestfulConfig struct {
	Host             string `yaml:"host"`
	CACert           string `yaml:"ca.crt,omitempty"`
	ClientCert       string `yaml:"client.crt,omitempty"`
	ClientKey        string `yaml:"client.key,omitempty"`
	CASecretName     string `yaml:"ca.secret,omitempty"`
	ClientSecretName string `yaml:"client.secret,omitempty"`
}

// YamlMarshal marshal the connection credential object, rawCert specifies whether to keep the cert in the data directly
func (k *RestfulConfig) YamlMarshal(rawCert bool) ([]byte, error) {
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

func (k *RestfulConfig) DeepCopy() *RestfulConfig {
	return &RestfulConfig{
		Host:             k.Host,
		CACert:           k.CACert,
		ClientCert:       k.ClientCert,
		ClientKey:        k.ClientKey,
		CASecretName:     k.CASecretName,
		ClientSecretName: k.ClientSecretName,
	}
}

func (k *RestfulConfig) GetCACert() string {
	return k.CACert
}

func (k *RestfulConfig) SetCACert(cert string) {
	k.CACert = cert
}

func (k *RestfulConfig) GetClientCert() string {
	return k.ClientCert
}

func (k *RestfulConfig) SetClientCert(cert string) {
	k.ClientCert = cert
}

func (k *RestfulConfig) GetClientKey() string {
	return k.ClientKey
}

func (k *RestfulConfig) SetClientKey(key string) {
	k.ClientKey = key
}

func (k *RestfulConfig) GetCASecretName() string {
	return k.CASecretName
}

func (k *RestfulConfig) GetClientSecretName() string {
	return k.ClientSecretName
}
