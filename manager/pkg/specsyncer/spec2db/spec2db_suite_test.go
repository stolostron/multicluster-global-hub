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

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testenv     *envtest.Environment
	cfg         *rest.Config
	ctx         context.Context
	cancel      context.CancelFunc
	testPostgre *TestPostgre
)

const (
	defaultUsername = "noroot"
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

	testPostgre, err = NewTestPostgre()
	Expect(err).NotTo(HaveOccurred())
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
	err = testPostgre.Stop()
	Expect(err).NotTo(HaveOccurred())
})

type TestPostgre struct {
	command  *exec.Cmd
	embedded *embeddedpostgres.EmbeddedPostgres
	rootUser bool
	URI      string
	username string
}

func NewTestPostgre() (*TestPostgre, error) {
	pg := &TestPostgre{}
	currentuser, err := user.Current()
	if err != nil {
		fmt.Printf("failed to get current user: %s", err.Error())
		return nil, err
	}
	if currentuser.Username == "root" {
		pg.rootUser = true
		pg.username = defaultUsername
		if pg.command, err = getPostgreCommand(defaultUsername); err != nil {
			fmt.Printf("failed to get PostgreCommand: %s", err.Error())
			return pg, err
		}
	} else {
		pg.rootUser = false
		pg.username = currentuser.Username
		pg.embedded = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Database("hoh"))
		if err = pg.embedded.Start(); err != nil {
			fmt.Printf("failed to get embeddedPostgre: %s", err.Error())
			return pg, err
		}
	}
	pg.URI = fmt.Sprintf("postgres://postgres:postgres@localhost:5432/%s?sslmode=disable", "hoh")
	return pg, nil
}

func (pg *TestPostgre) Stop() error {
	if pg.command != nil {
		if err := pg.command.Process.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("failed to terminate cmd processes: %s", err.Error())
			return err
		}

		// make sure the child process(postgre) is terminated
		if err := exec.Command("pkill", "-u", pg.username).Run(); err != nil {
			fmt.Printf("failed to terminate user(%s) processes: %s", pg.username, err.Error())
			return err
		}

		// delete the testuser
		if err := exec.Command("userdel", "-rf", pg.username).Run(); err != nil {
			fmt.Printf("failed to delete user(%s): %s", pg.username, err.Error())
			return err
		}
	}
	if pg.embedded != nil {
		if err := pg.embedded.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func getPostgreCommand(username string) (*exec.Cmd, error) {
	// create user
	_, err := user.Lookup(username)
	if err != nil && !strings.Contains(err.Error(), "unknown user") {
		fmt.Printf("failed to lookup user: %s", err.Error())
		return nil, err
	}
	if err != nil {
		if err = exec.Command("useradd", "-m", username).Run(); err != nil {
			fmt.Printf("failed to create user: %s", err.Error())
			return nil, err
		}
	}

	// grant privilege to the user
	err = exec.Command("usermod", "-G", "root", username).Run()
	if err != nil {
		fmt.Printf("failed to add permission to user: %s", err.Error())
		return nil, err
	}

	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("failed to get current dir: %s", err.Error())
		return nil, err
	}
	projectDir := strings.Replace(currentDir, "/manager/pkg/specsyncer/spec2db", "", 1)
	file := "test/pkg/postgre/main.go"
	goBytes, err := exec.Command("which", "go").Output()
	if err != nil {
		fmt.Printf("failed to get go binary dir: %s", err.Error())
		return nil, err
	}
	goBin := strings.Replace(string(goBytes), "\n", "", 1)
	cmd := exec.Command("su", "-c", fmt.Sprintf("cd %s && %s run %s", projectDir, goBin, file), "-", username)

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("failed to set stdout pipe: %s", err.Error())
		return cmd, err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("failed to set stderr pipe: %s", err.Error())
		return cmd, err
	}
	outPipeReader := bufio.NewReader(outPipe)
	errPipReader := bufio.NewReader(errPipe)

	err = cmd.Start()
	if err != nil {
		fmt.Printf("failed to start postgres command: %s", err.Error())
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
	case <-time.After(1 * time.Minute):
		return cmd, fmt.Errorf("waiting for database initialization timeout")
	}
}
