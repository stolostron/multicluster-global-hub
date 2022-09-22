package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	var postgresPort uint32 = 5432

	if len(os.Args) > 1 {
		intVar, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Printf("invalid port: %v\n", err)
			os.Exit(1)
		}
		if intVar < 1024 || intVar > 65535 {
			fmt.Println("invalid port value, should in the range of 1024 - 65535")
			os.Exit(1)
		}

		postgresPort = uint32(intVar)
	}

	fmt.Println(" # postgres process is running!")

	database := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(postgresPort).Database("hoh"))
	if err := database.Start(); err != nil {
		fmt.Printf("failed to start embedded postgres: %v", err)
		os.Exit(1)
	}

	fmt.Println(" # postgres started")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)

	ticker := time.NewTicker(time.Second)
	count := 0
loop:
	for {
		select {
		case sig := <-signalChan:
			fmt.Printf(" # [signal] %s %d \n", sig.String(), sig)
			break loop
		case <-ticker.C:
			count++
			fmt.Printf(" # (%d) ", count)
		}
	}

	if err := database.Stop(); err != nil {
		os.Exit(1)
	}
	fmt.Println(" # postgres process is ended!")
}
