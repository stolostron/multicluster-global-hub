package spec2db_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testenv        *envtest.Environment
	cfg            *rest.Config
	ctx            context.Context
	cancel         context.CancelFunc
	postgreCommand *exec.Cmd
	postgresURI    string
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

	postgreCommand, err = getPostgreCommand()
	Expect(err).NotTo(HaveOccurred())

	postgresURI = fmt.Sprintf("postgres://postgres:postgres@localhost:5432/%s", "hoh")
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

	postgreCommand.Process.Signal(syscall.SIGTERM)
	postgreCommand.Wait()
})

func getPostgreCommand() (*exec.Cmd, error) {
	testUser := "noroot"

	// create noroot user
	_, err := user.Lookup(testUser)
	if err != nil && strings.Contains(err.Error(), "unknown user") {
		_, err = exec.Command("useradd", "-m", testUser).Output()
		return nil, err
	} else if err != nil {
		return nil, err
	}

	// grant privilege to the user
	_, err = exec.Command("usermod", "-G", "root", testUser).Output()
	if err != nil {
		return nil, err
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	projectDir := strings.Replace(currentDir, "/manager/pkg/specsyncer/spec2db", "", 1)
	file := "test/pkg/postgre/main.go"
	cmd := exec.Command("su", "-c", fmt.Sprintf("cd %s && /usr/local/go/bin/go run %s", projectDir, file), "-", testUser)

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return cmd, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return cmd, err
	}
	outPipeReader := bufio.NewReader(outPipe)
	errPipReader := bufio.NewReader(errPipe)

	err = cmd.Start()
	if err != nil {
		return cmd, err
	}

	go func() {
		for {
			line, err := errPipReader.ReadString('\n')
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Printf("error reading file %s", err)
				break
			}
			fmt.Print(line)
		}
	}()

	postgreChan := make(chan string, 1)
	go func() {
		for {
			line, err := outPipeReader.ReadString('\n')
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Printf("error reading file %s", err)
				break
			}
			if strings.Contains(line, "postgres started") {
				postgreChan <- line
			} else {
				fmt.Print(line)
			}
		}
	}()

	// wait database to be ready
	select {
	case done := <-postgreChan:
		fmt.Printf("database: %s", done)
		return cmd, nil
	case <-time.After(10 * time.Second):
		return cmd, fmt.Errorf("waiting for database initialization timeout")
	}
}
