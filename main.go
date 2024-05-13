package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"

	cec "github.com/claes/cec"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var debug *bool

type CecMQTTBridge struct {
	MQTTClient    mqtt.Client
	CECConnection *cec.Connection
}

func NewCecMQTTBridge(cecName, cecDeviceName string, mqttBroker string) *CecMQTTBridge {

	fmt.Printf("Initializing CEC connection: %s %s: \n", cecName, cecDeviceName)

	cecConnection, err := cec.Open(cecName, cecDeviceName)
	if err != nil {
		fmt.Printf("Could not connect to CEC device %s %s, %v\n", cecName, cecDeviceName, err)
		panic(err)
	}

	fmt.Printf("CEC connection opened\n")

	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Printf("Could not connect to broker %s, %v\n", mqttBroker, token.Error())
		panic(token.Error())
	} else if *debug {
		fmt.Printf("Connected to MQTT broker: %s\n", mqttBroker)
	}

	bridge := &CecMQTTBridge{
		MQTTClient:    client,
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

func (bridge *CecMQTTBridge) MainLoop() {
}

func printHelp() {
	fmt.Println("Usage: cec-mqtt [OPTIONS]")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func main() {
	cecName := flag.String("cecName", "/dev/ttyACM0", "CEC name")
	cecDeviceName := flag.String("cecDeviceName", "Claes", "CEC device name")
	mqttBroker := flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	help := flag.Bool("help", false, "Print help")
	debug = flag.Bool("debug", false, "Debug logging")
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	bridge := NewCecMQTTBridge(*cecName, *cecDeviceName, *mqttBroker)

	go func() {
		bridge.CECConnection.Commands = make(chan *cec.Command, 10) // Buffered channel
		for command := range bridge.CECConnection.Commands {
			fmt.Printf("Command: %v \n", command.Operation)
		}
	}()

	go func() {
		bridge.CECConnection.KeyPresses = make(chan int, 10) // Buffered channel
		for keyPress := range bridge.CECConnection.KeyPresses {
			fmt.Printf("Key press: %v \n", keyPress)
			bridge.PublishMQTT("cec/key", strconv.Itoa(keyPress), false)
		}
	}()

	go func() {
		bridge.CECConnection.SourceActivations = make(chan *cec.SourceActivation, 10) // Buffered channel
		for sourceActivation := range bridge.CECConnection.SourceActivations {
			fmt.Printf("Source activation: %v %v\n", sourceActivation.LogicalAddress, sourceActivation.State)
			bridge.PublishMQTT("cec/source/"+strconv.Itoa(sourceActivation.LogicalAddress)+"/active",
				strconv.FormatBool(sourceActivation.State), true)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	fmt.Printf("Started\n")
	go bridge.MainLoop()
	<-c
	// bridge.Controller.Close()

	fmt.Printf("Shut down\n")
	bridge.CECConnection.Destroy()
	fmt.Printf("Exit\n")

	os.Exit(0)
}
