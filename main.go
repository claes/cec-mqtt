package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	lib "github.com/claes/cec-mqtt/lib"
)

var debug *bool

func MainLoop(bridge lib.CecMQTTBridge) {
	for {
		time.Sleep(10 * time.Second)
		bridge.CECConnection.Transmit("10:8F")
		time.Sleep(10 * time.Second)
	}
}

func printHelp() {
	fmt.Println("Usage: cec-mqtt [OPTIONS]")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func main() {
	cecName := flag.String("cecName", "/dev/ttyACM0", "CEC name")
	cecDeviceName := flag.String("cecDeviceName", "CEC-MQTT", "CEC device name")
	mqttBroker := flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	topicPrefix := flag.String("topicPrefix", "", "MQTT topic prefix")
	help := flag.Bool("help", false, "Print help")
	debug = flag.Bool("debug", false, "Debug logging")
	flag.Parse()

	if *debug {
		var programLevel = new(slog.LevelVar)
		programLevel.Set(slog.LevelDebug)
		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
		slog.SetDefault(slog.New(handler))
	}

	if *help {
		printHelp()
		os.Exit(0)
	}

	bridge := lib.NewCecMQTTBridge(lib.CreateCECConnection(*cecName, *cecDeviceName),
		lib.CreateMQTTClient(*mqttBroker), *topicPrefix)

	ctx := context.Background()
	go bridge.PublishCommands(ctx)
	go bridge.PublishKeyPresses(ctx)
	go bridge.PublishSourceActivations(ctx)
	go bridge.PublishMessages(ctx, true)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	slog.Info("Started")
	go MainLoop(*bridge)
	<-c
	// bridge.Controller.Close()

	slog.Info("Shut down")
	bridge.CECConnection.Destroy()
	slog.Info("Exit")

	os.Exit(0)
}
