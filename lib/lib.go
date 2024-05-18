package lib

import (
	"log/slog"
	"strconv"

	cec "github.com/claes/cec"
	mqtt "github.com/eclipse/paho.mqtt.golang"
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

func (bridge *CecMQTTBridge) PublishCommands() {
	bridge.CECConnection.Commands = make(chan *cec.Command, 10) // Buffered channel
	for command := range bridge.CECConnection.Commands {
		slog.Debug("Create command", "command", command.CommandString)
		bridge.PublishMQTT("cec/command", command.CommandString, false)
	}
}

func (bridge *CecMQTTBridge) PublishKeyPresses() {
	bridge.CECConnection.KeyPresses = make(chan *cec.KeyPress, 10) // Buffered channel
	for keyPress := range bridge.CECConnection.KeyPresses {
		slog.Debug("Key press", "keyCode", keyPress.KeyCode, "duration", keyPress.Duration)
		if keyPress.Duration == 0 {
			bridge.PublishMQTT("cec/key", strconv.Itoa(keyPress.KeyCode), false)
		}
	}
}

func (bridge *CecMQTTBridge) PublishSourceActivations() {
	bridge.CECConnection.SourceActivations = make(chan *cec.SourceActivation, 10) // Buffered channel
	for sourceActivation := range bridge.CECConnection.SourceActivations {
		slog.Debug("Source activation",
			"logicalAddress", sourceActivation.LogicalAddress,
			"state", sourceActivation.State)
		bridge.PublishMQTT("cec/source/"+strconv.Itoa(sourceActivation.LogicalAddress)+"/active",
			strconv.FormatBool(sourceActivation.State), true)
	}
}

func (bridge *CecMQTTBridge) PublishMessages(logOnly bool) {
	bridge.CECConnection.Messages = make(chan string, 10) // Buffered channel
	for message := range bridge.CECConnection.Messages {
		slog.Debug("Message", "message", message)
		if !logOnly {
			bridge.PublishMQTT("cec/message", message, false)
		}
	}
}
