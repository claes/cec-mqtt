package lib

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"sync"

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
	slog.Info("Creating MQTT client", "broker", mqttBroker)
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("Could not connect to broker", "mqttBroker", mqttBroker, "error", token.Error())
		panic(token.Error())
	}
	slog.Info("Connected to MQTT broker", "mqttBroker", mqttBroker)
	return client
}

func NewCecMQTTBridge(cecConnection *cec.Connection, mqttClient mqtt.Client) *CecMQTTBridge {

	slog.Info("Creating CEC MQTT bridge")
	bridge := &CecMQTTBridge{
		MQTTClient:    mqttClient,
		CECConnection: cecConnection,
	}

	funcs := map[string]func(client mqtt.Client, message mqtt.Message){
		"cec/key/send":   bridge.onKeySend,
		"cec/command/tx": bridge.onCommandSend,
	}
	for key, function := range funcs {
		token := mqttClient.Subscribe(key, 0, function)
		token.Wait()
	}

	bridge.initialize()
	slog.Info("CEC MQTT bridge initialized")
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

func (bridge *CecMQTTBridge) PublishCommands(ctx context.Context) {
	bridge.CECConnection.Commands = make(chan *cec.Command, 10) // Buffered channel
	for {
		select {
		case <-ctx.Done():
			slog.Info("PublishCommands function is being cancelled")
			return
		case command := <-bridge.CECConnection.Commands:
			slog.Debug("Create command", "command", command.CommandString)
			bridge.PublishMQTT("cec/command/rx", command.CommandString, false)
		}
	}
}

func (bridge *CecMQTTBridge) PublishKeyPresses(ctx context.Context) {
	bridge.CECConnection.KeyPresses = make(chan *cec.KeyPress, 10) // Buffered channel

	for {
		select {
		case <-ctx.Done():
			slog.Info("PublishKeyPresses function is being cancelled")
			return
		case keyPress := <-bridge.CECConnection.KeyPresses:
			slog.Debug("Key press", "keyCode", keyPress.KeyCode, "duration", keyPress.Duration)
			if keyPress.Duration == 0 {
				bridge.PublishMQTT("cec/key", strconv.Itoa(keyPress.KeyCode), false)
			}
		}
	}
}

func (bridge *CecMQTTBridge) PublishSourceActivations(ctx context.Context) {
	bridge.CECConnection.SourceActivations = make(chan *cec.SourceActivation, 10) // Buffered channel

	for {
		select {
		case <-ctx.Done():
			slog.Info("PublishCommands function is being cancelled")
			return
		case sourceActivation := <-bridge.CECConnection.SourceActivations:
			slog.Debug("Source activation",
				"logicalAddress", sourceActivation.LogicalAddress,
				"state", sourceActivation.State)
			bridge.PublishMQTT("cec/source/"+strconv.Itoa(sourceActivation.LogicalAddress)+"/active",
				strconv.FormatBool(sourceActivation.State), true)
		}
	}
}

func (bridge *CecMQTTBridge) PublishMessages(ctx context.Context, logOnly bool) {

	pattern := `^(>>|<<)\s([0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2})*)`
	regex, err := regexp.Compile(pattern)
	if err != nil {
		slog.Info("Error compiling regex", "error", err)
		return
	}

	bridge.CECConnection.Messages = make(chan string, 10) // Buffered channel
	for {
		select {
		case <-ctx.Done():
			slog.Info("PublishMessages function is being cancelled")
			return
		case message := <-bridge.CECConnection.Messages:
			slog.Debug("Message", "message", message)
			if !logOnly {
				bridge.PublishMQTT("cec/message", message, false)
			}
			matches := regex.FindStringSubmatch(message)
			if matches != nil {
				prefix := matches[1]
				hexPart := matches[2]
				slog.Debug("CEC Message payload match", "prefix", prefix, "hex", hexPart)
				if prefix == "<<" {
					bridge.PublishMQTT("cec/message/hex/rx", hexPart, true)
				} else if prefix == ">>" {
					bridge.PublishMQTT("cec/message/hex/tx", hexPart, true)
				}
			}
		}
	}
}

var sendMutex sync.Mutex

func (bridge *CecMQTTBridge) onCommandSend(client mqtt.Client, message mqtt.Message) {
	sendMutex.Lock()
	defer sendMutex.Unlock()

	if "" == string(message.Payload()) {
		return
	}
	command := string(message.Payload())
	if command != "" {
		bridge.PublishMQTT("cec/command/tx", "", false)
		slog.Debug("Sending command", "command", command)
		bridge.CECConnection.Transmit(command)
	}
}

func (bridge *CecMQTTBridge) onKeySend(client mqtt.Client, message mqtt.Message) {
	sendMutex.Lock()
	defer sendMutex.Unlock()

	if "" == string(message.Payload()) {
		return
	}
	var payload map[string]interface{}
	err := json.Unmarshal(message.Payload(), &payload)
	if err != nil {
		slog.Error("Could not parse payload", "payload", string(message.Payload()))
	}
	address := payload["address"].(float64)
	key := payload["key"].(string)
	if key != "" {
		bridge.PublishMQTT("cec/key/send", "", false)
		slog.Debug("Sending key", "address", address, "key", key)
		bridge.CECConnection.Key(int(address), key)
	}
}

// Create conditions to ping cec connection
// and to reconnect
