module github.com/stolostron/multicluster-global-hub

go 1.24.4

require (
	github.com/IBM/sarama v1.45.2
	github.com/RedHatInsights/strimzi-client-go v0.40.0
	github.com/authzed/spicedb-operator v1.20.1
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2 v2.0.0-20250904143103-af3e8599b331
	github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2 v2.15.2
	github.com/cloudevents/sdk-go/v2 v2.16.2
	github.com/cloudflare/cfssl v1.6.5
	github.com/confluentinc/confluent-kafka-go/v2 v2.11.1
	github.com/crunchydata/postgres-operator v1.3.3-0.20230629151007-94ebcf2df74d
	github.com/deckarep/golang-set v1.8.0
	github.com/evanphx/json-patch v5.9.11+incompatible
	github.com/fergusstrange/embedded-postgres v1.31.0
	github.com/gin-gonic/gin v1.10.1
	github.com/go-co-op/gocron v1.37.0
	github.com/go-kratos/kratos/v2 v2.8.4
	github.com/go-logr/logr v1.4.3
	github.com/go-logr/zapr v1.3.0
	github.com/gonvenience/ytbx v1.4.7
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/homeport/dyff v1.10.2
	github.com/lib/pq v1.10.9
	github.com/onsi/ginkgo/v2 v2.23.4
	github.com/onsi/gomega v1.38.0
	github.com/openshift/api v0.0.0-20250220103441-744790f2cff7
	github.com/openshift/client-go v0.0.0-20250131180035-f7ec47e2d87a
	github.com/openshift/library-go v0.0.0-20250228164547-bad2d1bf3a37
	github.com/operator-framework/api v0.33.0
	github.com/project-kessel/inventory-api v0.0.0-20241213103024-feb181fd66c1
	github.com/project-kessel/inventory-client-go v0.0.0-20240927104800-2c124202b25f
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.76.0
	github.com/spf13/pflag v1.0.7
	github.com/stolostron/cluster-lifecycle-api v0.0.0-20250429012240-363012f4f827
	github.com/stolostron/klusterlet-addon-controller v0.0.0-20250224012200-769f091c0e95
	github.com/stolostron/multicloud-operators-foundation v0.0.0-20241223014534-09421f48bba2
	github.com/stolostron/multiclusterhub-operator v0.0.0-20250415191038-1e368a726d8b
	github.com/stretchr/testify v1.11.0
	go.uber.org/zap v1.27.0
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/yaml.v2 v2.4.0
	gorm.io/datatypes v1.2.6
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.30.1
	k8s.io/api v0.33.4
	k8s.io/apiextensions-apiserver v0.33.4
	k8s.io/apimachinery v0.33.4
	k8s.io/client-go v0.33.4
	k8s.io/klog v1.0.0
	k8s.io/kube-aggregator v0.32.6
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397
	open-cluster-management.io/addon-framework v0.12.1-0.20250422083707-fb6b4ebb66b5
	open-cluster-management.io/api v1.0.0
	open-cluster-management.io/governance-policy-propagator v0.16.0
	open-cluster-management.io/managed-serviceaccount v0.8.0
	open-cluster-management.io/multicloud-operators-channel v0.16.0
	open-cluster-management.io/multicloud-operators-subscription v0.16.0
	sigs.k8s.io/application v0.8.3
	sigs.k8s.io/controller-runtime v0.21.0
	sigs.k8s.io/kustomize/api v0.20.1
	sigs.k8s.io/kustomize/kyaml v0.20.1
	sigs.k8s.io/yaml v1.5.0
)

require (
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/gonvenience/idem v0.0.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	go.yaml.in/yaml/v3 v3.0.3 // indirect
	helm.sh/helm/v3 v3.18.5 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.35.2-20240920164238-5a7b106cbb87.1 // indirect
	dario.cat/mergo v1.0.1 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/BurntSushi/toml v1.5.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.1 // indirect
	github.com/Masterminds/sprig/v3 v3.3.0 // indirect
	github.com/authzed/grpcutil v0.0.0-20240123194739-2ea1e3d2d98b // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bytedance/sonic v1.13.2 // indirect
	github.com/bytedance/sonic/loader v0.2.4 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.5 // indirect
	github.com/containerd/containerd/api v1.8.0 // indirect
	github.com/cyphar/filepath-securejoin v0.4.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/eapache/go-resiliency v1.7.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-kratos/aegis v0.2.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/go-playground/form/v4 v4.2.1 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gonvenience/bunt v1.4.2 // indirect
	github.com/gonvenience/neat v1.3.16 // indirect
	github.com/gonvenience/term v1.0.4 // indirect
	github.com/gonvenience/text v1.0.9 // indirect
	github.com/google/certificate-transparency-go v1.1.7 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/pprof v0.0.0-20250403155104-27863c87afa6 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.5
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jmoiron/sqlx v1.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect; indirec
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-ciede2000 v0.0.0-20170301095244-782e8c62fec3 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.22.0
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	github.com/virtuald/go-ordered-json v0.0.0-20170621173500-b18e6e673d74 // indirect
	github.com/weppos/publicsuffix-go v0.30.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/zmap/zcrypto v0.0.0-20230310154051-c8b263fd8300 // indirect
	github.com/zmap/zlint/v3 v3.5.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/arch v0.16.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/driver/mysql v1.5.6 // indirect
	k8s.io/apiserver v0.33.4 // indirect
	k8s.io/component-base v0.33.4 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250610211856-8b98d1ed966a // indirect
	open-cluster-management.io/sdk-go v0.16.0 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/kube-storage-version-migrator v0.0.6-0.20230721195810-5c8923c5ff96 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
)

replace github.com/elazarl/goproxy => github.com/elazarl/goproxy v0.0.0-20240726154733-8b0c20506380

// the multiclusterhub-operator uses controller-runtime under v0.20.*
// issue: https://github.com/stolostron/multiclusterhub-operator/issues/2186
replace sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.19.1
