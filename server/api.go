package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-plugin-starter-template/server/erpnext"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// ServeHTTP handles HTTP requests for the plugin.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := mux.NewRouter()

	// Don't try to use context, it's not needed
	apiRouter := router.PathPrefix("/api/v1").Subrouter()

	apiRouter.HandleFunc("/hello", p.HelloWorld).Methods(http.MethodGet)

	// Add admin-only middleware for the sync endpoint
	syncRouter := apiRouter.PathPrefix("/sync").Subrouter()
	syncRouter.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p.AdminAuthorizationRequired(w, r, next)
		})
	})
	syncRouter.HandleFunc("", p.SyncUsers).Methods(http.MethodPost)

	router.ServeHTTP(w, r)
}

// AdminAuthorizationRequired is middleware that checks if the user is a system admin
func (p *Plugin) AdminAuthorizationRequired(w http.ResponseWriter, r *http.Request, next http.Handler) {
	userID := r.Header.Get("Mattermost-User-ID")
	p.API.LogDebug("Received request with user ID", "user_id", userID)

	if userID == "" {
		p.API.LogError("User ID not found in request")
		http.Error(w, "Not authorized: missing user ID", http.StatusUnauthorized)
		return
	}

	user, appErr := p.API.GetUser(userID)
	if appErr != nil {
		p.API.LogError("Failed to get user", "error", appErr.Error())
		http.Error(w, "Not authorized: "+appErr.Error(), http.StatusUnauthorized)
		return
	}

	if !user.IsSystemAdmin() {
		p.API.LogError("User is not a system admin", "user_id", userID)
		http.Error(w, "Requires system admin privileges", http.StatusForbidden)
		return
	}

	// User is authorized, proceed
	next.ServeHTTP(w, r)
}

func (p *Plugin) HelloWorld(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("Hello, world!")); err != nil {
		p.API.LogError("Failed to write response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SyncUsers syncs Mattermost users with ERPNext employees
func (p *Plugin) SyncUsers(w http.ResponseWriter, r *http.Request) {
	// Log the start of function for debugging
	p.API.LogInfo("SyncUsers function started")

	if p.erpNextClient == nil {
		p.API.LogError("ERPNext client is not configured")
		http.Error(w, "ERPNext client is not configured properly. Please check the plugin settings.", http.StatusInternalServerError)
		return
	}

	// Check if the custom_chat_id field exists, and create it if it doesn't
	p.API.LogInfo("Checking if custom_chat_id field exists in ERPNext")

	exists, err := p.erpNextClient.CheckCustomFieldExists("custom_chat_id", "Employee")
	if err != nil {
		p.API.LogError("Failed to check if custom_chat_id field exists", "error", err)
		http.Error(w, fmt.Sprintf("Failed to check if custom_chat_id field exists: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if !exists {
		p.API.LogInfo("Creating custom_chat_id field in ERPNext")

		// Create the custom field
		err = p.erpNextClient.CreateCustomField(
			"custom_chat_id",     // Field name
			"Mattermost User ID", // Label
			"Employee",           // Document type
			"Data",               // Field type
			false,                // Not required
		)

		if err != nil {
			p.API.LogError("Failed to create custom_chat_id field", "error", err)
			http.Error(w, fmt.Sprintf("Failed to create custom_chat_id field: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		p.API.LogInfo("Successfully created custom_chat_id field in ERPNext")
	} else {
		p.API.LogInfo("custom_chat_id field already exists in ERPNext")
	}

	// Fetch all users from Mattermost
	p.API.LogInfo("Fetching Mattermost users")

	perPage := 200
	users, appErr := p.API.GetUsers(&model.UserGetOptions{
		Page:    0,
		PerPage: perPage,
		Active:  true, // Only fetch active (non-deleted) users
	})
	if appErr != nil {
		p.API.LogError("Failed to fetch users from Mattermost", "error", appErr.Error())
		http.Error(w, fmt.Sprintf("Failed to fetch users: %s", appErr.Error()), http.StatusInternalServerError)
		return
	}

	// Log summary of users fetched
	p.API.LogInfo(fmt.Sprintf("Fetched %d users from Mattermost", len(users)))

	// Build response data
	type SyncResult struct {
		MatchedCount int      `json:"matched_count"`
		UpdatedCount int      `json:"updated_count"`
		CreatedCount int      `json:"created_count"`
		SkippedCount int      `json:"skipped_count"`
		UserResults  []string `json:"user_results"`
	}

	result := SyncResult{
		UserResults: []string{},
	}

	// Process each user
	for _, user := range users {
		// Skip if user has no email
		if user.Email == "" {
			p.API.LogDebug("Skipping user with no email", "username", user.Username)
			result.SkippedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s (%s) - Skipped (No Email)", user.Username, user.Email))
			continue
		}

		// Skip if user is a bot
		if user.IsBot {
			p.API.LogDebug("Skipping bot user", "username", user.Username)
			result.SkippedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s (%s) - Skipped (Bot)", user.Username, user.Email))
			continue
		}

		// Skip if user is deleted
		if user.DeleteAt > 0 {
			p.API.LogDebug("Skipping deleted user", "username", user.Username, "deleteAt", user.DeleteAt)
			result.SkippedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s (%s) - Skipped (Deleted)", user.Username, user.Email))
			continue
		}

		// Try to find matching employee in ERPNext
		employee, err := p.erpNextClient.GetEmployeeByEmail(user.Email)
		if err != nil {
			p.API.LogError("Error finding employee by email",
				"email", user.Email,
				"error", err)
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s (%s) - Error: %s", user.Username, user.Email, err.Error()))
			continue
		}

		if employee != nil {
			// Employee found - check if we need to update the custom_chat_id
			if employee.CustomChatID != user.Id {
				// Need to update the custom_chat_id field
				p.API.LogInfo("Updating custom_chat_id for existing employee",
					"email", user.Email,
					"employee_id", employee.Name,
					"mattermost_id", user.Id)

				// Create an employee object with the updated custom_chat_id
				updatedEmployee := &erpnext.Employee{
					Name:         employee.Name,
					CustomChatID: user.Id,
				}

				// Call API to update the employee
				_, err := p.erpNextClient.UpdateEmployee(updatedEmployee)
				if err != nil {
					p.API.LogError("Failed to update employee custom_chat_id in ERPNext",
						"email", user.Email,
						"error", err)
					result.UserResults = append(result.UserResults,
						fmt.Sprintf("%s (%s) - Update Failed: %s", user.Username, user.Email, err.Error()))
					continue
				}

				result.UpdatedCount++
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Updated", user.Username, user.Email))
			} else {
				// Already mapped correctly
				result.MatchedCount++
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Already Mapped", user.Username, user.Email))
			}
		} else {
			// Employee not found - create a new one
			p.API.LogInfo("Creating new employee for Mattermost user",
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
			_, err := p.erpNextClient.CreateEmployee(newEmployee)
			if err != nil {
				p.API.LogError("Failed to create employee in ERPNext",
					"email", user.Email,
					"error", err)
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Creation Failed: %s", user.Username, user.Email, err.Error()))
				continue
			}

			result.CreatedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s (%s) - Created", user.Username, user.Email))
		}
	}

	// Create response summary
	summary := fmt.Sprintf(
		"Sync completed. Matched: %d, Updated: %d, Created: %d, Skipped: %d",
		result.MatchedCount, result.UpdatedCount, result.CreatedCount, result.SkippedCount,
	)
	p.API.LogInfo(summary)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		p.API.LogError("Failed to encode response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
