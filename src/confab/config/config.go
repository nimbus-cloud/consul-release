package config

import "encoding/json"

type Config struct {
	Node   ConfigNode
	Confab ConfigConfab
	Consul ConfigConsul
	Path   ConfigPath
}

type ConfigConfab struct {
	TimeoutInSeconds int `json:"timeout_in_seconds"`
}

type ConfigConsul struct {
	Agent       ConfigConsulAgent
	EncryptKeys []string `json:"encrypt_keys"`
}

type ConfigPath struct {
	AgentPath       string `json:"agent_path"`
	ConsulConfigDir string `json:"consul_config_dir"`
	PIDFile         string `json:"pid_file"`
	KeyringFile     string `json:"keyring_file"`
	DataDir         string `json:"data_dir"`
}

type ConfigNode struct {
	Name       string `json:"name"`
	Index      int    `json:"index"`
	ExternalIP string `json:"external_ip"`
}

type ConfigConsulAgent struct {
	Servers         ConfigConsulAgentServers     `json:"servers"`
	Services        map[string]ServiceDefinition `json:"services"`
	Mode            string                       `json:"mode"`
	Domain          string                       `json:"domain"`
	Datacenter      string                       `json:"datacenter"`
	LogLevel        string                       `json:"log_level"`
	ProtocolVersion int                          `json:"protocol_version"`
	DnsConfig       ConfigConsulAgentDnsConfig   `json:"dns_config"`
}

type ConfigConsulAgentDnsConfig struct {
	AllowStale bool   `json:"allow_stale"`
	MaxStale   string `json:"max_stale"`
}

type ConfigConsulAgentServers struct {
	LAN []string `json:"lan"`
	WAN []string `json:"wan"`
}

func defaultConfig() Config {
	return Config{
		Path: ConfigPath{
			AgentPath:       "/var/vcap/packages/consul/bin/consul",
			ConsulConfigDir: "/var/vcap/jobs/consul_agent/config",
			PIDFile:         "/var/vcap/sys/run/consul_agent/consul_agent.pid",
		},
		Consul: ConfigConsul{
			Agent: ConfigConsulAgent{
				DnsConfig: ConfigConsulAgentDnsConfig{
					AllowStale: false,
					MaxStale:   "5s",
				},
				Servers: ConfigConsulAgentServers{
					LAN: []string{},
					WAN: []string{},
				},
			},
		},
		Confab: ConfigConfab{
			TimeoutInSeconds: 55,
		},
	}
}

func ConfigFromJSON(configData []byte) (Config, error) {
	config := defaultConfig()

	if err := json.Unmarshal(configData, &config); err != nil {
		return Config{}, err
	}

	if config.Path.KeyringFile == "" {
		if config.Consul.Agent.Mode == "server" {
			config.Path.KeyringFile = "/var/vcap/store/consul_agent/serf/local.keyring"
		} else {
			config.Path.KeyringFile = "/var/vcap/data/consul_agent/serf/local.keyring"
		}
	}

	if config.Path.DataDir == "" {
		if config.Consul.Agent.Mode == "server" {
			config.Path.DataDir = "/var/vcap/store/consul_agent"
		} else {
			config.Path.DataDir = "/var/vcap/data/consul_agent"
		}
	}

	return config, nil

}
