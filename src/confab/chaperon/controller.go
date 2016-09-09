package chaperon

import (
	"errors"
	"time"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/consul-release/src/confab"
	"github.com/cloudfoundry-incubator/consul-release/src/confab/agent"
	"github.com/cloudfoundry-incubator/consul-release/src/confab/config"
	consulagent "github.com/hashicorp/consul/command/agent"
)

type stopper interface {
	Stop() error
}

type agentRunner interface {
	Run() error
	Stop() error
	Wait() error
	Cleanup() error
	WritePID() error
}

type agentClient interface {
	VerifyJoined() error
	VerifySynced() error
	IsLastNode() (bool, error)
	SetKeys([]string) error
	Leave() error
	SetConsulRPCClient(agent.ConsulRPCClient)
}

type serviceDefiner interface {
	GenerateDefinitions(config.Config) []config.ServiceDefinition
	WriteDefinitions(string, []config.ServiceDefinition) error
}

type clock interface {
	Sleep(time.Duration)
}

type logger interface {
	Info(action string, data ...lager.Data)
	Error(action string, err error, data ...lager.Data)
}

type Controller struct {
	AgentRunner    agentRunner
	AgentClient    agentClient
	SyncRetryDelay time.Duration
	SyncRetryClock clock
	EncryptKeys    []string
	SSLDisabled    bool
	Logger         logger
	ConfigDir      string
	ServiceDefiner serviceDefiner
	Config         config.Config
}

func (c Controller) BootAgent(timeout confab.Timeout) error {
	c.Logger.Info("controller.boot-agent.run")
	err := c.AgentRunner.Run()
	if err != nil {
		c.Logger.Error("controller.boot-agent.run.failed", err)
		return err
	}

	c.Logger.Info("controller.boot-agent.verify-joined")

	if err := c.callWithTimeout(timeout, c.AgentClient.VerifyJoined); err != nil {
		c.Logger.Error("controller.boot-agent.verify-joined.failed", err)
		return err
	}

	c.Logger.Info("controller.boot-agent.success")
	return nil
}

func (c Controller) callWithTimeout(timeout confab.Timeout, f func() error) error {
	for {
		select {
		case <-timeout.Done():
			return errors.New("timeout exceeded")
		default:
			err := f()
			if err != nil {
				c.SyncRetryClock.Sleep(c.SyncRetryDelay)
				continue
			}

			return nil
		}
	}
}

func (c Controller) ConfigureServer(timeout confab.Timeout, rpcClient *consulagent.RPCClient) error {
	if rpcClient != nil {
		c.AgentClient.SetConsulRPCClient(&agent.RPCClient{*rpcClient})
	}

	c.Logger.Info("controller.configure-server.is-last-node")
	lastNode, err := c.AgentClient.IsLastNode()
	if err != nil {
		c.Logger.Error("controller.configure-server.is-last-node.failed", err)
		return err
	}

	if lastNode {
		c.Logger.Info("controller.configure-server.verify-synced")
		if err := c.callWithTimeout(timeout, c.AgentClient.VerifySynced); err != nil {
			c.Logger.Error("controller.configure-server.verify-synced.failed", err)
			return err
		}
	}

	if len(c.EncryptKeys) == 0 {
		err := errors.New("encrypt keys cannot be empty if ssl is enabled")
		c.Logger.Error("controller.configure-server.no-encrypt-keys", err)
		return err
	}

	c.Logger.Info("controller.configure-server.set-keys", lager.Data{
		"keys": c.EncryptKeys,
	})

	err = c.AgentClient.SetKeys(c.EncryptKeys)
	if err != nil {
		c.Logger.Error("controller.configure-server.set-keys.failed", err, lager.Data{
			"keys": c.EncryptKeys,
		})
		return err
	}

	if err := c.AgentRunner.WritePID(); err != nil {
		c.Logger.Error("controller.configure-server.write-pid.failed", err)
		return err
	}

	c.Logger.Info("controller.configure-server.success")
	return nil
}

func (c Controller) ConfigureClient() error {
	err := c.AgentRunner.WritePID()
	if err != nil {
		return err
	}

	return nil
}

func (c Controller) StopAgent(rpcClient *consulagent.RPCClient) {
	if rpcClient != nil {
		c.AgentClient.SetConsulRPCClient(&agent.RPCClient{*rpcClient})
	}

	c.Logger.Info("controller.stop-agent.leave")
	if err := c.AgentClient.Leave(); err != nil {
		c.Logger.Error("controller.stop-agent.leave.failed", err)

		c.Logger.Info("controller.stop-agent.stop")
		if err = c.AgentRunner.Stop(); err != nil {
			c.Logger.Error("controller.stop-agent.stop.failed", err)
		}
	}

	c.Logger.Info("controller.stop-agent.wait")
	if err := c.AgentRunner.Wait(); err != nil {
		c.Logger.Error("controller.stop-agent.wait.failed", err)
	}

	c.Logger.Info("controller.stop-agent.cleanup")
	if err := c.AgentRunner.Cleanup(); err != nil {
		c.Logger.Error("controller.stop-agent.cleanup.failed", err)
	}

	c.Logger.Info("controller.stop-agent.success")
}

func (c Controller) WriteServiceDefinitions() error {
	c.Logger.Info("controller.write-service-definitions.generate-definitions")
	definitions := c.ServiceDefiner.GenerateDefinitions(c.Config)

	c.Logger.Info("controller.write-service-definitions.write")
	if err := c.ServiceDefiner.WriteDefinitions(c.ConfigDir, definitions); err != nil {
		c.Logger.Error("controller.write-service-definitions.write.failed", err)
		return err
	}

	c.Logger.Info("controller.write-service-definitions.success")
	return nil
}
