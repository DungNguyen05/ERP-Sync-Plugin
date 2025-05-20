package command

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-starter-template/server/erpnext"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// Constants for command triggers
const (
	HelloCommandTrigger    = "hello"
	EmployeeCommandTrigger = "employee"
	MapUsersCommandTrigger = "mapusers" // New command
)

// Handler implements the Command interface
type Handler struct {
	client        *pluginapi.Client
	erpNextClient *erpnext.Client
}

// Command interface defines the methods that need to be implemented by command handlers
type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
	SetERPNextClient(client *erpnext.Client)
}

// NewCommandHandler creates and registers slash commands
func NewCommandHandler(client *pluginapi.Client) Command {
	handler := &Handler{
		client: client,
	}

	// Register hello command
	err := client.SlashCommand.Register(&model.Command{
		Trigger:          HelloCommandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Say hello to someone",
		AutoCompleteHint: "[@username]",
		AutocompleteData: model.NewAutocompleteData(HelloCommandTrigger, "[@username]", "Username to say hello to"),
	})
	if err != nil {
		client.Log.Error("Failed to register hello command", "error", err)
	}

	// Register employee command
	err = client.SlashCommand.Register(&model.Command{
		Trigger:          EmployeeCommandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Get the total number of employees from ERPNext",
		DisplayName:      "Employee Count",
		Description:      "Fetches the total number of employees from ERPNext",
	})
	if err != nil {
		client.Log.Error("Failed to register employee command", "error", err)
	}

	// Register mapusers command
	err = client.SlashCommand.Register(&model.Command{
		Trigger:          MapUsersCommandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Map Mattermost users to ERPNext employees by email",
		DisplayName:      "Map Users",
		Description:      "Fetches all users from Mattermost and maps them to ERPNext employees by email",
	})
	if err != nil {
		client.Log.Error("Failed to register mapusers command", "error", err)
	}

	return handler
}

// SetERPNextClient sets the ERPNext client for the command handler
func (h *Handler) SetERPNextClient(client *erpnext.Client) {
	h.erpNextClient = client
}

// Handle processes slash commands
func (h *Handler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	trigger := strings.TrimPrefix(strings.Fields(args.Command)[0], "/")

	switch trigger {
	case HelloCommandTrigger:
		return h.executeHelloCommand(args), nil
	case EmployeeCommandTrigger:
		return h.executeEmployeeCommand(args), nil
	case MapUsersCommandTrigger:
		return h.executeMapUsersCommand(args), nil
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: %s", args.Command),
		}, nil
	}
}

// executeHelloCommand handles the /hello command
func (h *Handler) executeHelloCommand(args *model.CommandArgs) *model.CommandResponse {
	if len(strings.Fields(args.Command)) < 2 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Please specify a username",
		}
	}
	username := strings.Fields(args.Command)[1]
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         "Hello, " + username,
	}
}

// executeEmployeeCommand handles the /employee command
func (h *Handler) executeEmployeeCommand(args *model.CommandArgs) *model.CommandResponse {
	// Check if ERPNext client is configured
	if h.erpNextClient == nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "ERPNext client is not configured properly. Please check the plugin settings.",
		}
	}

	// Fetch employees from ERPNext
	employees, err := h.erpNextClient.GetEmployees()
	if err != nil {
		h.client.Log.Error("Failed to fetch employees from ERPNext", "error", err)
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Failed to fetch employees: %s", err.Error()),
		}
	}

	// Return the employee count
	employeeCount := len(employees)
	var response string

	if employeeCount == 0 {
		response = "No employees found in ERPNext."
	} else if employeeCount == 1 {
		response = "There is 1 employee in ERPNext."
	} else {
		response = fmt.Sprintf("There are %d employees in ERPNext.", employeeCount)
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         response,
	}
}

// executeMapUsersCommand handles the /mapusers command
func (h *Handler) executeMapUsersCommand(args *model.CommandArgs) *model.CommandResponse {
	// Check if ERPNext client is configured
	if h.erpNextClient == nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "ERPNext client is not configured properly. Please check the plugin settings.",
		}
	}

	// Fetch all users from Mattermost
	perPage := 200
	users, err := h.client.User.List(&model.UserGetOptions{
		Page:    0,
		PerPage: perPage,
		Active:  true,
	})
	if err != nil {
		h.client.Log.Error("Failed to fetch users from Mattermost", "error", err)
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Failed to fetch users: %s", err.Error()),
		}
	}

	// Build response
	var matchedCount int
	var responseBuilder strings.Builder
	responseBuilder.WriteString("### Mattermost Users Mapped to ERPNext\n\n")
	responseBuilder.WriteString("| Mattermost Username | Email | ERPNext Employee ID |\n")
	responseBuilder.WriteString("|-------------------|-------|-------------------|\n")

	// Process each user
	for _, user := range users {
		// Skip if user has no email
		if user.Email == "" {
			continue
		}

		// Try to find matching employee in ERPNext
		employee, err := h.erpNextClient.GetEmployeeByEmail(user.Email)
		if err != nil {
			h.client.Log.Error("Error finding employee by email", "email", user.Email, "error", err)
			continue
		}

		if employee != nil {
			matchedCount++
			responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s |\n", user.Username, user.Email, employee.Name))
		}
	}

	// If no matches found
	if matchedCount == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "No Mattermost users matched with ERPNext employees by email.",
		}
	}

	// Add summary
	responseBuilder.WriteString(fmt.Sprintf("\n**Total matched users:** %d", matchedCount))

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseBuilder.String(),
	}
}
