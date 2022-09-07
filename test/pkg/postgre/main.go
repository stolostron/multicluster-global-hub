package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	fmt.Println(" # postgres process is running!")

	database := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Database("hoh"))
	err := database.Start()
	fmt.Println(" # postgres started")

	if err != nil {
		os.Exit(1)
	}

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

	if err = database.Stop(); err != nil {
		os.Exit(1)
	}
	fmt.Println(" # postgres process is ended!")
}
