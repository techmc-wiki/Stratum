package config

type Controller struct {
	ListenAddress string
	DataDirectory string
}
type Agent struct {
	ControllerAddress string
	AgentID           string
	DataDirectory     string
	ListenAddress     string
	Token             string
	RuntimeMode       string
	RuntimeRoot       string
}
