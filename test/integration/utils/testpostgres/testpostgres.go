// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package testpostgres

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"
)

type TestPostgres struct {
	command  *exec.Cmd
	embedded *embeddedpostgres.EmbeddedPostgres
	rootUser bool
	URI      string
	username string
}

func NewTestPostgres() (*TestPostgres, error) {
	pg := &TestPostgres{}
	currentUser, err := user.Current()
	if err != nil {
		fmt.Printf("failed to get current user: %s", err.Error())
		return nil, err
	}

	// generate random postgres port
	postgresPort := uint32(rand.Intn(65535-1024) + 1024)
	for !isPortAvailable(postgresPort) {
		postgresPort = uint32(rand.Intn(65535-1024) + 1024)
	}

	// if the current is root user, then it creates a non-root user and start a postgres process
	if currentUser.Username == "root" {
		pg.rootUser = true
		// unit tests are running in parallel, test user name can't conflict
		pg.username = fmt.Sprintf("non-root-%d", postgresPort)
		if pg.command, err = getPostgresCommand(pg.username, postgresPort); err != nil {
			fmt.Printf("failed to get PostgresCommand: %s", err.Error())
			return pg, err
		}
	} else {
		postgresDataPath, err := os.UserHomeDir()
		if err != nil || postgresDataPath == "" {
			postgresDataPath = os.TempDir()
		}
		postgresDataPath = filepath.Join(postgresDataPath,
			fmt.Sprintf("/tmp/embedded-postgres-go-%d", postgresPort),
			"extracted")
		pg.rootUser = false
		pg.username = currentUser.Username

		if _, err := os.Stat(postgresDataPath); os.IsNotExist(err) {
			if err := os.MkdirAll(postgresDataPath, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create directory %s: %w", postgresDataPath, err)
			}
		}

		postgresConfig := embeddedpostgres.DefaultConfig().
			Port(postgresPort).
			RuntimePath(postgresDataPath).
			BinariesPath(postgresDataPath).
			DataPath(filepath.Join(postgresDataPath, "data")).
			Database("hoh")

		pg.embedded = embeddedpostgres.NewDatabase(postgresConfig)
		if err = pg.embedded.Start(); err != nil {
			fmt.Printf("failed to get embeddedPostgres: %s", err.Error())
			return pg, err
		}
	}
	pg.URI = fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", postgresPort, "hoh")
	return pg, nil
}

func (pg *TestPostgres) Stop() error {
	if pg.command != nil {
		if err := pg.command.Process.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("failed to terminate cmd processes: %s", err.Error())
			return err
		}

		// make sure the child process(postgres) is terminated
		if err := exec.Command("pkill", "-u", pg.username).Run(); err != nil {
			fmt.Printf("failed to terminate user(%s) processes: %s", pg.username, err.Error())
			return err
		}

		// delete the testUser
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

func getPostgresCommand(username string, postgresPort uint32) (*exec.Cmd, error) {
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

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Printf("failed to get current dir: no caller information")
		return nil, fmt.Errorf("failed to get current dir: no caller information")
	}
	rootDir := strings.Replace(filename, "test/integration/utils/testpostgres/testpostgres.go", "", 1)
	file := "test/integration/utils/testpostgres/cmd/main.go"
	goBytes, err := exec.Command("which", "go").Output()
	if err != nil {
		fmt.Printf("failed to get go binary dir: %s", err.Error())
		return nil, err
	}
	goBin := strings.Replace(string(goBytes), "\n", "", 1)
	cmd := exec.Command("su", "-c", fmt.Sprintf("cd %s && %s run %s %d", rootDir, goBin, file, postgresPort), "-", username)

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

	postgresChan := make(chan string, 1)
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
				postgresChan <- line
			} else {
				fmt.Print(line)
			}
		}
	}()

	// wait database to be ready
	select {
	case done := <-postgresChan:
		fmt.Printf("database: %s", done)
		return cmd, nil
	case <-time.After(1 * time.Minute):
		return cmd, fmt.Errorf("waiting for database initialization timeout")
	}
}

func isPortAvailable(port uint32) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	defer func() {
		if err := listener.Close(); err != nil {
			log.Printf("failed to close listener: %v", err)
		}
	}()
	return true
}
