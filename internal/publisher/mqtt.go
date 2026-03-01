package publisher

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	connectTimeout  = 10 * time.Second
	publishTimeout  = 5 * time.Second
	disconnectQuiesce = 250 // ms; time allowed for in-flight messages before disconnect
)

// MQTT publishes to an MQTT broker via the Paho client library.
type MQTT struct {
	client mqtt.Client
}

// Config holds MQTT connection parameters.
type Config struct {
	BrokerURL string
	ClientID  string
	Username  string
	Password  string
}

// NewMQTT connects to the broker described by cfg and returns a ready Publisher.
// It will retry connections automatically on transient disconnects.
func NewMQTT(cfg Config) (*MQTT, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.BrokerURL)
	opts.SetClientID(cfg.ClientID)
	opts.SetConnectTimeout(connectTimeout)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(2 * time.Second)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	// Last Will: mark daemon as offline if connection drops unexpectedly.
	opts.SetWill(
		fmt.Sprintf("tempest/%s/daemon/status", cfg.ClientID),
		`{"online":false}`,
		1,
		true,
	)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(connectTimeout) {
		return nil, fmt.Errorf("mqtt connect timeout after %s", connectTimeout)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("mqtt connect to %q: %w", cfg.BrokerURL, err)
	}

	return &MQTT{client: client}, nil
}

// Publish sends payload to topic with qos and retain settings.
func (p *MQTT) Publish(topic string, payload []byte, qos byte, retain bool) error {
	token := p.client.Publish(topic, qos, retain, payload)
	if !token.WaitTimeout(publishTimeout) {
		return fmt.Errorf("mqtt publish timeout to %q", topic)
	}
	return token.Error()
}

// Disconnect gracefully shuts down the MQTT client, allowing in-flight messages
// to complete.
func (p *MQTT) Disconnect() {
	p.client.Disconnect(disconnectQuiesce)
}
