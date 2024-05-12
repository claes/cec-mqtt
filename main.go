package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

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

	return bridge
}

func (bridge *CecMQTTBridge) PublishMQTT(topic string, message string, retained bool) {
	token := bridge.MQTTClient.Publish(topic, 0, retained, message)
	token.Wait()
}

func (bridge *CecMQTTBridge) MainLoop() {
	//fmt.Printf("Set active source\n")
	//bridge.CECConnection.SetActiveSource(1)
	for {
		time.Sleep(10 * time.Second)
		fmt.Printf("List sources again\n")
		cecDevices := bridge.CECConnection.List()
		for key, value := range cecDevices {
			fmt.Printf("   %s: Active source %v, Logical Adress %v, OSD name %v, Physical Adress %v, Power %v, Vendor %v\n", key, value.ActiveSource,
				value.LogicalAddress, value.OSDName, value.PhysicalAddress, value.PowerStatus, value.Vendor)
		}
		time.Sleep(10 * time.Second)
		//fmt.Printf("Rescan devices\n")
		//bridge.CECConnection.RescanDevices()
	}
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

	// go func() {
	// 	for {
	// 		time.Sleep(8 * time.Second)
	// 		bridge.reconnectIfNeeded()
	// 	}
	// }()

	go func() {
		bridge.CECConnection.KeyPresses = make(chan int, 10) // Buffered channel
		for keyPress := range bridge.CECConnection.KeyPresses {
			fmt.Printf("Key press: %v \n", keyPress)
		}
	}()

	go func() {
		bridge.CECConnection.SourceActivations = make(chan *cec.SourceActivation, 10) // Buffered channel
		for sourceActivation := range bridge.CECConnection.SourceActivations {
			fmt.Printf("Source activation: %v %v\n", sourceActivation.LogicalAddress, sourceActivation.State)
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
