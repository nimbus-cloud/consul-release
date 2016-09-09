package chaperon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/consul-release/src/confab/chaperon"
	"github.com/cloudfoundry-incubator/consul-release/src/confab/config"
	"github.com/cloudfoundry-incubator/consul-release/src/confab/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/pivotal-cf-experimental/gomegamatchers"
)

var _ = Describe("ConfigWriter", func() {
	var (
		configDir string
		dataDir   string
		cfg       config.Config
		writer    chaperon.ConfigWriter
		logger    *fakes.Logger
	)

	Describe("Write", func() {
		BeforeEach(func() {
			logger = &fakes.Logger{}

			var err error
			configDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			dataDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())

			cfg = config.Config{}
			cfg.Consul.Agent.DnsConfig.MaxStale = "5s"
			cfg.Node = config.ConfigNode{Name: "node", Index: 0}
			cfg.Path.ConsulConfigDir = configDir
			cfg.Path.DataDir = dataDir

			writer = chaperon.NewConfigWriter(configDir, logger)
		})

		It("writes a config file to the consul_config dir", func() {
			err := writer.Write(cfg)
			Expect(err).NotTo(HaveOccurred())

			buf, err := ioutil.ReadFile(filepath.Join(configDir, "config.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(buf).To(MatchJSON(fmt.Sprintf(`{
				"server": false,
				"domain": "",
				"datacenter": "",
				"data_dir": %q,
				"log_level": "",
				"node_name": "node-0",
				"ports": {
					"dns": 53
				},
				"rejoin_after_leave": true,
				"retry_join": [],
				"retry_join_wan": [],
				"bind_addr": "",
				"disable_remote_exec": true,
				"disable_update_check": true,
				"protocol": 0,
				"verify_outgoing": true,
				"verify_incoming": true,
				"verify_server_hostname": true,
				"ca_file": "%[2]s/certs/ca.crt",
				"key_file": "%[2]s/certs/agent.key",
				"cert_file": "%[2]s/certs/agent.crt",
				"dns_config": {
				  "allow_stale": false,
				  "max_stale": "5s"
				}
			}`, dataDir, configDir)))

			Expect(logger.Messages()).To(ContainSequence([]fakes.LoggerMessage{
				{
					Action: "config-writer.write.generate-configuration",
				},
				{
					Action: "config-writer.write.write-file",
					Data: []lager.Data{{
						"config": config.GenerateConfiguration(cfg, configDir, "node-0"),
					}},
				},
				{
					Action: "config-writer.write.success",
				},
			}))
		})

		Context("node name", func() {
			Context("when node-name.json does not exist", func() {
				It("uses the job name-index and writes node-name.json", func() {
					err := writer.Write(cfg)
					Expect(err).NotTo(HaveOccurred())

					buf, err := ioutil.ReadFile(filepath.Join(dataDir, "node-name.json"))
					Expect(err).NotTo(HaveOccurred())

					Expect(buf).To(MatchJSON(`{"node_name":"node-0"}`))

					buf, err = ioutil.ReadFile(filepath.Join(configDir, "config.json"))
					Expect(err).NotTo(HaveOccurred())

					var config map[string]interface{}

					err = json.Unmarshal(buf, &config)
					Expect(err).NotTo(HaveOccurred())
					Expect(config["node_name"]).To(Equal("node-0"))

					Expect(logger.Messages()).To(ContainSequence([]fakes.LoggerMessage{
						{
							Action: "config-writer.write.determine-node-name",
							Data: []lager.Data{{
								"node-name": "node-0",
							}},
						},
					}))
				})
			})

			Context("when node-name.json exists", func() {
				It("uses the the name from the file", func() {
					err := ioutil.WriteFile(filepath.Join(dataDir, "node-name.json"),
						[]byte(`{"node_name": "some-node-name"}`), os.ModePerm)
					Expect(err).NotTo(HaveOccurred())

					err = writer.Write(cfg)
					Expect(err).NotTo(HaveOccurred())

					buf, err := ioutil.ReadFile(filepath.Join(dataDir, "node-name.json"))
					Expect(err).NotTo(HaveOccurred())

					Expect(buf).To(MatchJSON(`{"node_name":"some-node-name"}`))

					buf, err = ioutil.ReadFile(filepath.Join(configDir, "config.json"))
					Expect(err).NotTo(HaveOccurred())

					var config map[string]interface{}

					err = json.Unmarshal(buf, &config)
					Expect(err).NotTo(HaveOccurred())
					Expect(config["node_name"]).To(Equal("some-node-name"))
				})
			})

			Context("failure cases", func() {
				It("logs errors", func() {
					cfg.Path.DataDir = "/some/fake/path"
					writer.Write(cfg)

					Expect(logger.Messages()).To(ContainSequence([]fakes.LoggerMessage{
						{
							Action: "config-writer.write.determine-node-name.failed",
							Error:  errors.New("stat /some/fake/path: no such file or directory"),
						},
					}))
				})

				It("returns an error when the data dir does not exist", func() {
					cfg.Path.DataDir = "/some/fake/path"

					err := writer.Write(cfg)
					Expect(err).To(MatchError("stat /some/fake/path: no such file or directory"))
				})

				It("returns an error when node-name.json has malformed json", func() {
					err := ioutil.WriteFile(filepath.Join(dataDir, "node-name.json"),
						[]byte(`%%%%%`), os.ModePerm)
					Expect(err).NotTo(HaveOccurred())

					err = writer.Write(cfg)
					Expect(err).To(MatchError("invalid character '%' looking for beginning of value"))
				})

				It("returns an error when node-name.json cannot be written to", func() {
					err := os.Chmod(dataDir, 0555)
					Expect(err).NotTo(HaveOccurred())

					err = writer.Write(cfg)
					Expect(err).To(MatchError(ContainSubstring("node-name.json: permission denied")))
				})

				It("returns an error when node-name.json cannot be read", func() {
					err := ioutil.WriteFile(filepath.Join(dataDir, "node-name.json"),
						[]byte(`%%%%%`), 0)
					Expect(err).NotTo(HaveOccurred())

					err = writer.Write(cfg)
					Expect(err).To(MatchError(ContainSubstring("node-name.json: permission denied")))
				})
			})
		})

		Context("failure cases", func() {
			It("returns an error when the config file can't be written to", func() {
				err := os.Chmod(configDir, 0000)
				Expect(err).NotTo(HaveOccurred())

				err = writer.Write(cfg)
				Expect(err).To(MatchError(ContainSubstring("permission denied")))

				Expect(logger.Messages()).To(ContainSequence([]fakes.LoggerMessage{
					{
						Action: "config-writer.write.generate-configuration",
					},
					{
						Action: "config-writer.write.write-file",
						Data: []lager.Data{{
							"config": config.GenerateConfiguration(cfg, configDir, "node-0"),
						}},
					},
					{
						Action: "config-writer.write.write-file.failed",
						Error:  fmt.Errorf("open %s: permission denied", filepath.Join(configDir, "config.json")),
					},
				}))
			})
		})
	})
})
