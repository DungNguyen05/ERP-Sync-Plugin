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
	h.client.Log.Info("MapUsers command started", "user", args.UserId)

	// Check if ERPNext client is configured
	if h.erpNextClient == nil {
		h.client.Log.Error("ERPNext client is not configured")
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeInChannel,
			Text:         "ERPNext client is not configured properly. Please check the plugin settings.",
		}
	}

	// First, check if the custom_chat_id field exists, and create it if it doesn't
	h.client.Log.Info("Checking if custom_chat_id field exists in ERPNext")

	exists, err := h.erpNextClient.CheckCustomFieldExists("custom_chat_id", "Employee")
	if err != nil {
		h.client.Log.Error("Failed to check if custom_chat_id field exists", "error", err)
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeInChannel,
			Text:         fmt.Sprintf("Failed to check if custom_chat_id field exists: %s", err.Error()),
		}
	}

	if !exists {
		h.client.Log.Info("Creating custom_chat_id field in ERPNext")

		// Create the custom field
		err = h.erpNextClient.CreateCustomField(
			"custom_chat_id",     // Field name
			"Mattermost User ID", // Label
			"Employee",           // Document type
			"Data",               // Field type
			false,                // Not required
		)

		if err != nil {
			h.client.Log.Error("Failed to create custom_chat_id field", "error", err)
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeInChannel,
				Text:         fmt.Sprintf("Failed to create custom_chat_id field: %s", err.Error()),
			}
		}

		h.client.Log.Info("Successfully created custom_chat_id field in ERPNext")
	} else {
		h.client.Log.Info("custom_chat_id field already exists in ERPNext")
	}

	// Continue with the existing code to fetch and process users
	h.client.Log.Info("Fetching Mattermost users")

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
			ResponseType: model.CommandResponseTypeInChannel,
			Text:         fmt.Sprintf("Failed to fetch users: %s", err.Error()),
		}
	}

	// Build response
	var matchedCount int
	var updatedCount int
	var createdCount int
	var skippedCount int
	var responseBuilder strings.Builder
	responseBuilder.WriteString("### Mattermost Users Mapped to ERPNext\n\n")
	responseBuilder.WriteString("| Mattermost Username | Email | First Name | Last Name | ERPNext Employee ID | Status |\n")
	responseBuilder.WriteString("|-------------------|-------|------------|-----------|-------------------|--------|\n")

	// Process each user
	for _, user := range users {
		// Skip if user has no email
		if user.Email == "" {
			h.client.Log.Debug("Skipping user with no email", "username", user.Username)
			skippedCount++
			continue
		}

		// Skip if user is a bot
		if user.IsBot {
			h.client.Log.Debug("Skipping bot user", "username", user.Username)
			skippedCount++
			responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | - | Skipped (Bot) |\n",
				user.Username,
				user.Email,
				user.FirstName,
				user.LastName))
			continue
		}

		// Try to find matching employee in ERPNext
		employee, err := h.erpNextClient.GetEmployeeByEmail(user.Email)
		if err != nil {
			h.client.Log.Error("Error finding employee by email",
				"email", user.Email,
				"error", err)
			continue
		}

		if employee != nil {
			// Employee found - check if we need to update the custom_chat_id
			if employee.CustomChatID != user.Id {
				// Need to update the custom_chat_id field
				h.client.Log.Info("Updating custom_chat_id for existing employee",
					"email", user.Email,
					"employee_id", employee.Name,
					"mattermost_id", user.Id)

				// Create an employee object with the updated custom_chat_id
				updatedEmployee := &erpnext.Employee{
					Name:         employee.Name,
					CustomChatID: user.Id,
				}

				// Call API to update the employee
				_, err := h.erpNextClient.UpdateEmployee(updatedEmployee)
				if err != nil {
					h.client.Log.Error("Failed to update employee custom_chat_id in ERPNext",
						"email", user.Email,
						"error", err)
					responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | Update Failed |\n",
						user.Username,
						user.Email,
						user.FirstName,
						user.LastName,
						employee.Name))
					continue
				}

				updatedCount++
				responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | Updated |\n",
					user.Username,
					user.Email,
					user.FirstName,
					user.LastName,
					employee.Name))
			} else {
				// Already mapped correctly
				matchedCount++
				responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | Already Mapped |\n",
					user.Username,
					user.Email,
					user.FirstName,
					user.LastName,
					employee.Name))
			}
		} else {
			// Employee not found - create a new one
			h.client.Log.Info("Creating new employee for Mattermost user",
				"username", user.Username,
				"email", user.Email)

			// Create new employee with fixed values as specified
			newEmployee := &erpnext.Employee{
				CompanyEmail:  user.Email,
				FirstName:     user.FirstName,
				LastName:      user.LastName,
				Gender:        "Male",       // Fixed as specified
				DateOfBirth:   "2000-01-01", // Fixed as specified
				DateOfJoining: "2000-01-01", // Fixed as specified
				Status:        "Active",
				CustomChatID:  user.Id, // Store Mattermost ID
			}

			// Call API to create the employee
			createdEmployee, err := h.erpNextClient.CreateEmployee(newEmployee)
			if err != nil {
				h.client.Log.Error("Failed to create employee in ERPNext",
					"email", user.Email,
					"error", err)
				responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | Error | Failed to create |\n",
					user.Username,
					user.Email,
					user.FirstName,
					user.LastName))
				continue
			}

			createdCount++
			responseBuilder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | Created |\n",
				user.Username,
				user.Email,
				user.FirstName,
				user.LastName,
				createdEmployee.Name))
		}
	}

	// If no matches, updates or creations
	if matchedCount == 0 && updatedCount == 0 && createdCount == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeInChannel,
			Text:         "No Mattermost users processed. Check the logs for errors.",
		}
	}

	// Add summary
	responseBuilder.WriteString(fmt.Sprintf("\n**Total already mapped users:** %d  \n**Total updated users:** %d  \n**Total created users:** %d  \n**Total skipped users:** %d",
		matchedCount, updatedCount, createdCount, skippedCount))

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         responseBuilder.String(),
	}
}
