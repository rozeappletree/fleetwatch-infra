package fleetbridge

// MQTTValidationError is returned when an incoming MQTT payload passes JSON
// parsing but fails semantic validation (e.g. zero lat/lng).
type MQTTValidationError struct {
	Message string
}

func (e MQTTValidationError) Error() string {
	return e.Message
}
