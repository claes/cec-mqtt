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

	//fmt.Printf("Set active source\n")
	//cecConnection.SetActiveSource(1)

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

	// funcs := map[string]func(client mqtt.Client, message mqtt.Message){
	// 	"samsungremote/key/send":          bridge.onKeySend,
	// 	"samsungremote/key/reconnectsend": bridge.onKeyReconnectSend,
	// }
	// for key, function := range funcs {
	// 	token := client.Subscribe(key, 0, function)
	// 	token.Wait()
	// }
	// time.Sleep(2 * time.Second)
	return bridge
}

// var reconnectSamsungTV = false

// func (bridge *SamsungRemoteMQTTBridge) reconnectIfNeeded() {
// 	if reconnectSamsungTV {
// 		err := bridge.Controller.Connect(bridge.NetworkInfo, bridge.TVInfo)
// 		if *debug {
// 			if err != nil {
// 				fmt.Printf("Could not reconnect, %v\n", err)
// 			} else {
// 				fmt.Printf("Reconnection successful\n")
// 				reconnectSamsungTV = false
// 			}
// 		}
// 	}
// }

// var sendMutex sync.Mutex

// func (bridge *SamsungRemoteMQTTBridge) onKeySend(client mqtt.Client, message mqtt.Message) {
// 	sendMutex.Lock()
// 	defer sendMutex.Unlock()

// 	command := string(message.Payload())
// 	if command != "" {
// 		bridge.PublishMQTT("samsungremote/key/send", "", false)
// 		if *debug {
// 			fmt.Printf("Sending key %s\n", command)
// 		}
// 		err := bridge.Controller.SendKey(bridge.NetworkInfo, bridge.TVInfo, command)
// 		if err != nil {
// 			if *debug {
// 				fmt.Printf("Error while sending key, attempt reconnect\n")
// 			}
// 			reconnectSamsungTV = true
// 		}
// 	}
// }

// func (bridge *SamsungRemoteMQTTBridge) onKeyReconnectSend(client mqtt.Client, message mqtt.Message) {
// 	sendMutex.Lock()
// 	defer sendMutex.Unlock()

// 	command := string(message.Payload())
// 	if command != "" {
// 		bridge.PublishMQTT("samsungremote/key/reconnectsend", "", false)
// 		if *debug {
// 			fmt.Printf("Sending key %s\n", command)
// 		}
// 		reconnectSamsungTV = true
// 		bridge.reconnectIfNeeded()
// 		bridge.Controller.SendKey(bridge.NetworkInfo, bridge.TVInfo, command)
// 	}
// }

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
		fmt.Printf("Printing keypresses\n")
		for keyPress := range bridge.CECConnection.KeyPresses {
			fmt.Printf("KeyPress: %v \n", keyPress)
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
