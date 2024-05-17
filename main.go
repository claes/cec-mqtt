package main

import (
	"flag"
	"fmt"
	cec "github.com/claes/cec"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"time"
)

var debug *bool

type CecMQTTBridge struct {
	MQTTClient    mqtt.Client
	CECConnection *cec.Connection
}

func CreateCECConnection(cecName, cecDeviceName string) *cec.Connection {
	slog.Info("Initializing CEC connection", "cecName", cecName, "cecDeviceName", cecDeviceName)

	cecConnection, err := cec.Open(cecName, cecDeviceName)
	if err != nil {
		slog.Error("Could not connect to CEC device",
			"cecName", cecName, "cecDeviceName", cecDeviceName, "error", err)
		panic(err)
	}

	slog.Info("CEC connection opened")
	return cecConnection
}

func CreateMQTTClient(mqttBroker string) mqtt.Client {
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("Could not connect to broker", "mqttBroker", mqttBroker, "error", token.Error())
		panic(token.Error())
	} else if *debug {
		slog.Debug("Connected to MQTT broker", "mqttBroker", mqttBroker)
	}
	return client
}

func NewCecMQTTBridge(cecConnection *cec.Connection, mqttClient mqtt.Client) *CecMQTTBridge {

	bridge := &CecMQTTBridge{
		MQTTClient:    mqttClient,
		CECConnection: cecConnection,
	}
	bridge.initialize()
	return bridge
}

func (bridge *CecMQTTBridge) initialize() {
	cecDevices := bridge.CECConnection.List()
	for key, value := range cecDevices {
		slog.Info("Connected device",
			"key", key,
			"activeSource", value.ActiveSource,
			"logicalAddress", value.LogicalAddress,
			"osdName", value.OSDName,
			"physicalAddress", value.PhysicalAddress,
			"powerStatus", value.PowerStatus,
			"vendor", value.Vendor)
		bridge.PublishMQTT("cec/source/"+strconv.Itoa(value.LogicalAddress)+"/active",
			strconv.FormatBool(value.ActiveSource), true)
		bridge.PublishMQTT("cec/source/"+strconv.Itoa(value.LogicalAddress)+"/name",
			value.OSDName, true)
		bridge.PublishMQTT("cec/source/"+strconv.Itoa(value.LogicalAddress)+"/power",
			value.PowerStatus, true)
	}
}

func (bridge *CecMQTTBridge) PublishMQTT(topic string, message string, retained bool) {
	token := bridge.MQTTClient.Publish(topic, 0, retained, message)
	token.Wait()
}

func (bridge *CecMQTTBridge) publishCommand() {
	bridge.CECConnection.Commands = make(chan *cec.Command, 10) // Buffered channel
	for command := range bridge.CECConnection.Commands {
		slog.Debug("Create command", "command", command.CommandString)
		bridge.PublishMQTT("cec/command", command.CommandString, false)
	}
}

func (bridge *CecMQTTBridge) publishKeyPress() {
	bridge.CECConnection.KeyPresses = make(chan *cec.KeyPress, 10) // Buffered channel
	for keyPress := range bridge.CECConnection.KeyPresses {
		slog.Debug("Key press", "keyCode", keyPress.KeyCode, "duration", keyPress.Duration)
		if keyPress.Duration == 0 {
			bridge.PublishMQTT("cec/key", strconv.Itoa(keyPress.KeyCode), false)
		}
	}
}

func (bridge *CecMQTTBridge) publishSourceActivation() {
	bridge.CECConnection.SourceActivations = make(chan *cec.SourceActivation, 10) // Buffered channel
	for sourceActivation := range bridge.CECConnection.SourceActivations {
		slog.Debug("Source activation",
			"logicalAddress", sourceActivation.LogicalAddress,
			"state", sourceActivation.State)
		bridge.PublishMQTT("cec/source/"+strconv.Itoa(sourceActivation.LogicalAddress)+"/active",
			strconv.FormatBool(sourceActivation.State), true)
	}
}

func (bridge *CecMQTTBridge) publishMessage(logOnly bool) {
	bridge.CECConnection.Messages = make(chan string, 10) // Buffered channel
	for message := range bridge.CECConnection.Messages {
		slog.Debug("Message", "message", message)
		if !logOnly {
			bridge.PublishMQTT("cec/message", message, false)
		}
	}
}

func (bridge *CecMQTTBridge) MainLoop() {
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

	bridge := NewCecMQTTBridge(CreateCECConnection(*cecName, *cecDeviceName),
		CreateMQTTClient(*mqttBroker))

	go bridge.publishCommand()
	go bridge.publishKeyPress()
	go bridge.publishSourceActivation()
	go bridge.publishMessage(true)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	slog.Info("Started")
	go bridge.MainLoop()
	<-c
	// bridge.Controller.Close()

	slog.Info("Shut down")
	bridge.CECConnection.Destroy()
	slog.Info("Exit")

	os.Exit(0)
}
