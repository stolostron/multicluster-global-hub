package spec2db_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testenv          *envtest.Environment
	cfg              *rest.Config
	ctx              context.Context
	cancel           context.CancelFunc
	postgreContainer tc.Container
	postgresURI      string
)

func TestSpec2db(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spec2db Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

	var err error
	testenv = &envtest.Environment{}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	postgreContainer, postgresURI = getPostgreContainer(ctx)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	err = testenv.Stop()
	Expect(err).NotTo(HaveOccurred())

	postgreContainer.Terminate(ctx)
})

func getPostgreContainer(ctx context.Context) (tc.Container, string) {
	// Prepare the database with container-go: https://golang.testcontainers.org/quickstart/gotest
	user := "user"
	password := "pass"
	database := "hoh"

	workingDir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	databaseDir := strings.Replace(workingDir, "manager/pkg/specsyncer/spec2db", "operator/pkg/controllers/hubofhubs/database", 1)

	postgresPort := nat.Port("5432/tcp")
	postgresContainer, err := tc.GenericContainer(context.Background(),
		tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				Image:        "postgres:14.1-alpine",
				ExposedPorts: []string{postgresPort.Port()},
				Env: map[string]string{
					"POSTGRES_PASSWORD": password,
					"POSTGRES_USER":     user,
					"POSTGRES_DB":       database,
				},
				Mounts: tc.ContainerMounts{{
					Source: tc.GenericBindMountSource{
						HostPath: databaseDir,
					},
					Target:   "/docker-entrypoint-initdb.d",
					ReadOnly: true,
				}},
				WaitingFor: wait.ForAll(
					wait.ForLog("database system is ready to accept connections"),
					wait.ForListeningPort(postgresPort),
				),
			},
			Started: true,
		})

	Expect(err).NotTo(HaveOccurred())

	hostPort, err := postgresContainer.MappedPort(ctx, postgresPort)
	Expect(err).NotTo(HaveOccurred())
	postgresURI := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s", user, password, hostPort.Port(), database)
	return postgresContainer, postgresURI
}
