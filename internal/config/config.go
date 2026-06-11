package config

type Controller struct {
	ListenAddress string
	DataDirectory string
}
type Agent struct {
	ControllerAddress string
	AgentID           string
	DataDirectory     string
}
