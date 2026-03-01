// Package publisher provides an abstraction over MQTT publishing.
// The Publisher interface allows the daemon to be tested without a real broker.
package publisher

// Publisher sends MQTT messages.
type Publisher interface {
	// Publish sends payload to topic with the given QoS level and retain flag.
	Publish(topic string, payload []byte, qos byte, retain bool) error
}
