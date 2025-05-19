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
