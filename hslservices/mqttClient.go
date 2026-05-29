package fleetbridge

import (
	"encoding/json"
	"fmt"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	log "github.com/sirupsen/logrus"
)

// Environment variables (set in envs/mqtt_connector.env or docker-compose):
//
//	MQTT_TOPIC   trucks/#                         ← subscribe all truck topics
//	MQTT_BROKER  192.168.31.116  or  emqx         ← broker host
//	MQTT_PORT    1883                              ← plain TCP (no TLS for internal)
var (
	mqttTopic      = os.Getenv("MQTT_TOPIC")  // e.g. "trucks/#"
	mqttBrokerHost = os.Getenv("MQTT_BROKER") // broker hostname or IP
	mqttPort       = os.Getenv("MQTT_PORT")   // "1883"
)

// MsgBroker is a simple fan-out staging channel.
// All MQTT messages land here; a pool of Redis workers drain it.
type MsgBroker struct {
	StagingC chan []byte
}

// NewMsgBroker creates a buffered MsgBroker with capacity n.
func NewMsgBroker(n int) *MsgBroker {
	return &MsgBroker{
		StagingC: make(chan []byte, n),
	}
}

// messageHandler implements mqtt.MessageHandler.
// It is invoked by the paho library on every received message and must be
// goroutine-safe and non-blocking.
func (mb *MsgBroker) messageHandler(client mqtt.Client, msg mqtt.Message) {
	select {
	case mb.StagingC <- msg.Payload():
		log.WithField("Topic", msg.Topic()).Debug("Msg Recv")
	default:
		log.WithField("Topic", msg.Topic()).Warn("Staging channel full — msg dropped")
	}
}

// connectHandler is called once the MQTT client establishes a session.
// Subscriptions are re-applied here so they survive broker restarts.
func connectHandler(handler mqtt.MessageHandler) mqtt.OnConnectHandler {
	return func(client mqtt.Client) {
		token := client.Subscribe(mqttTopic, 1, handler)
		token.Wait()
		if token.Error() != nil {
			log.WithError(token.Error()).Error("MQTT subscription failed")
			return
		}

		log.WithField("Topic", mqttTopic).Info("Subscribed to broker topic")
	}
}

// connectionLostHandler logs unexpected disconnections.
func connectionLostHandler(client mqtt.Client, err error) {
	log.Warnf("MQTT connection lost: %v — client will auto-reconnect", err)
}

// InitMQTTClient creates and connects a paho MQTT client pointing at the
// internal EMQX broker (plain TCP, not TLS — use TLS on port 8883 in
// production via a reverse-proxy or stunnel).
func InitMQTTClient(StgC *MsgBroker) *mqtt.Client {
	opts := mqtt.NewClientOptions()

	// Plain TCP — our own EMQX broker runs inside the Docker network / LAN.
	// Switch to "mqtts://" and add TLS config for production deployments.
	opts.AddBroker(
		fmt.Sprintf("tcp://%s:%s", mqttBrokerHost, mqttPort),
	)

	opts.SetClientID("fleet-go-worker")
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)
	opts.SetAutoReconnect(true)
	opts.SetDefaultPublishHandler(StgC.messageHandler)
	opts.SetOnConnectHandler(connectHandler(StgC.messageHandler))
	opts.SetConnectionLostHandler(connectionLostHandler)

	client := mqtt.NewClient(opts)

	log.WithFields(log.Fields{
		"Broker":        fmt.Sprintf("tcp://%s:%s", mqttBrokerHost, mqttPort),
		"Topic":         mqttTopic,
		"AutoReconnect": opts.AutoReconnect,
	}).Info("Connecting to MQTT broker")

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Panic(token.Error())
	}

	return &client
}

// DeserializeMQTTBody unmarshals a raw MQTT payload JSON into an EventHolder.
// The Flutter app publishes TruckEvent JSON directly (no wrapper), so we
// unmarshal into the VP field directly via the EventHolder shim.
//
// Returns MQTTValidationError if lat/lng are zero (bad GPS fix or tunnel).
func DeserializeMQTTBody(msgb []byte, hold *EventHolder) error {
	// The Flutter app sends TruckEvent JSON directly — unmarshal into VP
	if err := json.Unmarshal(msgb, &hold.VP); err != nil {
		return err
	}

	if hold.VP.Lat == 0.0 || hold.VP.Lng == 0.0 {
		return &MQTTValidationError{"Missing or zero coordinates — discarding"}
	}

	return nil
}
