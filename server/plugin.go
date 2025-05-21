package main

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
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

// GenerateUsername creates a slug from first and last name
// It removes special characters and spaces, converts to lowercase,
// and transforms Vietnamese and other accented characters to ASCII equivalents
func (p *Plugin) GenerateUsername(firstName, lastName string) string {
	// Combine first and last name
	fullName := firstName
	if lastName != "" {
		fullName += "." + lastName
	}

	// Convert to lowercase
	fullName = strings.ToLower(fullName)

	// Replace Vietnamese and other accented characters with ASCII equivalents
	fullName = p.removeAccents(fullName)

	// Replace special characters and spaces with underscores
	reg := regexp.MustCompile(`[^a-z0-9]`)
	username := reg.ReplaceAllString(fullName, "_")

	// Remove consecutive underscores
	regConsecutive := regexp.MustCompile(`_+`)
	username = regConsecutive.ReplaceAllString(username, "_")

	// Trim leading and trailing underscores
	username = strings.Trim(username, "_")

	// If username is empty, generate a random one
	if username == "" {
		username = "user_" + p.randomString(6)
	}

	// Ensure username is at least 3 characters
	for len(username) < 3 {
		username += "_" + p.randomString(3)
	}

	// Limit username length to 22 characters (Mattermost limit is 64, but keeping it shorter)
	if len(username) > 22 {
		username = username[:22]
	}

	return username
}

// removeAccents replaces Vietnamese and other accented characters with their ASCII equivalents
func (p *Plugin) removeAccents(s string) string {
	// Map of accented characters to their ASCII equivalents
	replacements := map[string]string{
		// Vietnamese vowels with diacritics
		"à": "a", "á": "a", "ạ": "a", "ả": "a", "ã": "a",
		"â": "a", "ầ": "a", "ấ": "a", "ậ": "a", "ẩ": "a", "ẫ": "a",
		"ă": "a", "ằ": "a", "ắ": "a", "ặ": "a", "ẳ": "a", "ẵ": "a",
		"è": "e", "é": "e", "ẹ": "e", "ẻ": "e", "ẽ": "e",
		"ê": "e", "ề": "e", "ế": "e", "ệ": "e", "ể": "e", "ễ": "e",
		"ì": "i", "í": "i", "ị": "i", "ỉ": "i", "ĩ": "i",
		"ò": "o", "ó": "o", "ọ": "o", "ỏ": "o", "õ": "o",
		"ô": "o", "ồ": "o", "ố": "o", "ộ": "o", "ổ": "o", "ỗ": "o",
		"ơ": "o", "ờ": "o", "ớ": "o", "ợ": "o", "ở": "o", "ỡ": "o",
		"ù": "u", "ú": "u", "ụ": "u", "ủ": "u", "ũ": "u",
		"ư": "u", "ừ": "u", "ứ": "u", "ự": "u", "ử": "u", "ữ": "u",
		"ỳ": "y", "ý": "y", "ỵ": "y", "ỷ": "y", "ỹ": "y",
		"đ": "d",

		// Other common accented characters
		"ç": "c", "ñ": "n", "ü": "u", "ö": "o", "ä": "a",
		"ß": "ss", "ø": "o", "å": "a", "æ": "ae", "œ": "oe",
		"ğ": "g", "ş": "s", "ı": "i", "ţ": "t", "ț": "t",
		"ș": "s", "ř": "r", "č": "c", "ě": "e", "š": "s",
		"ň": "n", "ď": "d", "ť": "t", "ĺ": "l", "ľ": "l",
		"ź": "z", "ż": "z", "ć": "c", "ń": "n", "ą": "a",
		"ę": "e", "ł": "l", "ő": "o", "ű": "u", "ž": "z",
		"ů": "u", "ā": "a", "ē": "e", "ī": "i", "ū": "u",
		"ģ": "g", "ķ": "k", "ļ": "l", "ņ": "n", "ŗ": "r",
	}

	// Apply replacements
	for accented, ascii := range replacements {
		s = strings.ReplaceAll(s, accented, ascii)
	}

	return s
}

// randomString generates a random string of specified length
func (p *Plugin) randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}

	return string(b)
}

// GenerateRandomPassword creates a random password with the specified length
// including uppercase, lowercase, numbers, and special characters
func (p *Plugin) GenerateRandomPassword(length int) string {
	if length < 8 {
		length = 8 // Enforce minimum length for security
	}

	const charsetLower = "abcdefghijklmnopqrstuvwxyz"
	const charsetUpper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const charsetNumber = "0123456789"
	const charsetSpecial = "!@#$%^&*()-_=+[]{}|;:,.<>?"

	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Ensure at least one of each character type
	password := []byte{
		charsetLower[seededRand.Intn(len(charsetLower))],
		charsetUpper[seededRand.Intn(len(charsetUpper))],
		charsetNumber[seededRand.Intn(len(charsetNumber))],
		charsetSpecial[seededRand.Intn(len(charsetSpecial))],
	}

	// Fill the rest with random characters from all charsets
	allCharset := charsetLower + charsetUpper + charsetNumber + charsetSpecial
	for i := 4; i < length; i++ {
		password = append(password, allCharset[seededRand.Intn(len(allCharset))])
	}

	// Shuffle the password characters
	seededRand.Shuffle(len(password), func(i, j int) {
		password[i], password[j] = password[j], password[i]
	})

	return string(password)
}

// SendCredentialEmail attempts to send an email to the user with their login credentials
// Returns true if the email was successfully sent, false otherwise
func (p *Plugin) SendCredentialEmail(email, username, password string) bool {
	// Get site URL from config
	config := p.API.GetConfig()
	if config.ServiceSettings.SiteURL == nil || *config.ServiceSettings.SiteURL == "" {
		p.API.LogError("Failed to get site URL from config")
		return false
	}
	siteURL := *config.ServiceSettings.SiteURL

	// Format email body
	subject := "Your Mattermost Account"
	bodyTemplate := `
Hello,

An account has been created for you on Mattermost. Here are your login details:

Site: %s
Username: %s
Password: %s

Please log in and change your password at your earliest convenience.

This is an automated message.
`
	body := fmt.Sprintf(bodyTemplate, siteURL, username, password)

	// Send email
	err := p.API.SendMail(email, subject, body)

	if err != nil {
		p.API.LogError("Failed to send credential email", "email", email, "error", err.Error())
		return false
	}

	p.API.LogInfo("Credential email sent successfully", "email", email)
	return true
}
