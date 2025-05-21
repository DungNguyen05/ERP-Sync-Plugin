package main

import (
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-starter-template/server/erpnext"
	"github.com/mattermost/mattermost-plugin-starter-template/server/store/kvstore"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// kvstore is the client used to read/write KV records for this plugin.
	kvstore kvstore.KVStore

	// client is the Mattermost server API client.
	client *pluginapi.Client

	// erpNextClient is the client used to interact with ERPNext API.
	erpNextClient *erpnext.Client

	backgroundJob *cluster.Job

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	// Initialize the Mattermost API client
	p.client = pluginapi.NewClient(p.API, p.Driver)

	// Initialize the KV store client
	p.kvstore = kvstore.NewKVStore(p.client)

	// Initialize the ERPNext client based on configuration
	config := p.getConfiguration()
	if config.ERPNextURL != "" && config.ERPNextAPIKey != "" && config.ERPNextAPISecret != "" {
		p.erpNextClient = erpnext.NewClient(
			config.ERPNextURL,
			config.ERPNextAPIKey,
			config.ERPNextAPISecret,
		)
	} else {
		p.API.LogInfo("ERPNext client not initialized: configuration missing. This is expected on first startup.")
	}

	// Schedule the background job
	job, err := cluster.Schedule(
		p.API,
		"BackgroundJob",
		cluster.MakeWaitForRoundedInterval(1*time.Hour),
		p.runJob,
	)
	if err != nil {
		return errors.Wrap(err, "failed to schedule background job")
	}

	p.backgroundJob = job

	return nil
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	p.setConfiguration(configuration)

	// Update the ERPNext client when configuration changes
	if configuration.ERPNextURL != "" && configuration.ERPNextAPIKey != "" && configuration.ERPNextAPISecret != "" {
		p.erpNextClient = erpnext.NewClient(
			configuration.ERPNextURL,
			configuration.ERPNextAPIKey,
			configuration.ERPNextAPISecret,
		)
	} else {
		p.API.LogInfo("ERPNext client not initialized: configuration missing")
		p.erpNextClient = nil
	}

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	if p.backgroundJob != nil {
		if err := p.backgroundJob.Close(); err != nil {
			p.API.LogError("Failed to close background job", "err", err)
		}
	}
	return nil
}
