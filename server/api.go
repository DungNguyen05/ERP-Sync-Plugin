package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

	// Add admin-only middleware for the sync endpoints
	syncRouter := apiRouter.PathPrefix("/sync").Subrouter()
	syncRouter.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p.AdminAuthorizationRequired(w, r, next)
		})
	})

	// Sync endpoints with descriptive paths
	syncRouter.HandleFunc("/mm-to-erp", p.SyncUsers).Methods(http.MethodPost)
	syncRouter.HandleFunc("/erp-to-mm", p.SyncEmployees).Methods(http.MethodPost)

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

// SyncUsers syncs Mattermost users with ERPNext employees and creates ERPNext users
func (p *Plugin) SyncUsers(w http.ResponseWriter, r *http.Request) {
	// Log the start of function for debugging
	p.API.LogInfo("SyncUsers function started")

	// Add timeout protection for large syncs
	startTime := time.Now()
	maxDuration := 15 * time.Minute // Increased timeout for large syncs

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
			"custom_chat_id",   // Field name
			"Workdone User ID", // Label
			"Employee",         // Document type
			"Data",             // Field type
			false,              // Not required
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

	// Check if the "Mặc định" role profile exists, and create it if it doesn't
	p.API.LogInfo("Checking if 'Mặc định' role profile exists in ERPNext")

	roleProfileExists, err := p.erpNextClient.CheckRoleProfileExists("Mặc định")
	if err != nil {
		p.API.LogError("Failed to check if 'Mặc định' role profile exists", "error", err)
		http.Error(w, fmt.Sprintf("Failed to check if 'Mặc định' role profile exists: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if !roleProfileExists {
		p.API.LogInfo("Creating 'Mặc định' role profile in ERPNext")

		err = p.erpNextClient.CreateRoleProfile("Mặc định")
		if err != nil {
			p.API.LogError("Failed to create 'Mặc định' role profile", "error", err)
			http.Error(w, fmt.Sprintf("Failed to create 'Mặc định' role profile: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		p.API.LogInfo("Successfully created 'Mặc định' role profile in ERPNext")
	} else {
		p.API.LogInfo("'Mặc định' role profile already exists in ERPNext")
	}

	// Fetch all users from Mattermost with pagination
	p.API.LogInfo("Fetching Mattermost users with pagination")

	perPage := 200
	var allUsers []*model.User
	page := 0

	// Fetch all users with pagination
	for {
		users, appErr := p.API.GetUsers(&model.UserGetOptions{
			Page:    page,
			PerPage: perPage,
			Active:  true, // Only fetch active (non-deleted) users
		})
		if appErr != nil {
			p.API.LogError("Failed to fetch users from Mattermost", "error", appErr.Error(), "page", page)
			http.Error(w, fmt.Sprintf("Failed to fetch users: %s", appErr.Error()), http.StatusInternalServerError)
			return
		}

		// Add users to our collection
		allUsers = append(allUsers, users...)

		p.API.LogInfo(fmt.Sprintf("Fetched page %d: %d users (total so far: %d)", page+1, len(users), len(allUsers)))

		// If we got fewer users than the page size, we've reached the end
		if len(users) < perPage {
			break
		}

		page++

		// Safety check to prevent infinite loops (allows up to 2000 users)
		if page > 15 { // Increased limit: 15 pages * 200 per page = 3000 users max
			p.API.LogWarn("Reached maximum page limit during user sync", "pages_fetched", page)
			break
		}
	}

	// Use allUsers for the rest of the function
	users := allUsers

	// Log summary of users fetched
	p.API.LogInfo(fmt.Sprintf("Fetched %d total users from Mattermost across %d pages", len(users), page+1))

	// Build response data
	type SyncResult struct {
		MatchedCount    int      `json:"matched_count"`
		UpdatedCount    int      `json:"updated_count"`
		CreatedCount    int      `json:"created_count"`
		SkippedCount    int      `json:"skipped_count"`
		ERPUsersCreated int      `json:"erp_users_created"`
		ERPUsersAlready int      `json:"erp_users_already_exist"`
		UserResults     []string `json:"user_results"`
		TotalProcessed  int      `json:"total_processed"`
		TimedOut        bool     `json:"timed_out"`
	}

	result := SyncResult{
		UserResults: []string{},
	}

	// Process each user
	for i, user := range users {
		// Check for timeout
		if time.Since(startTime) > maxDuration {
			p.API.LogWarn("Sync operation reached maximum duration, stopping", "processed_users", i)
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("TIMEOUT: Sync stopped after processing %d users due to timeout", i))
			result.TimedOut = true
			break
		}

		// Progress logging for large syncs
		if i > 0 && i%50 == 0 {
			p.API.LogInfo(fmt.Sprintf("Sync progress: processed %d/%d users (%.1f%%)",
				i, len(users), float64(i)/float64(len(users))*100))
		}

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

		var isNewEmployee bool = false

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
			} else {
				// Already mapped correctly
				result.MatchedCount++
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
			isNewEmployee = true
		}

		// Now check if ERPNext user exists for this employee
		p.API.LogInfo("Checking if ERPNext user exists for employee", "email", user.Email)

		erpUser, err := p.erpNextClient.GetUserByEmail(user.Email)
		if err != nil {
			p.API.LogError("Error checking ERPNext user by email", "email", user.Email, "error", err)
			// Continue with the next user instead of failing completely
			if isNewEmployee {
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Employee Created, User Check Failed: %s", user.Username, user.Email, err.Error()))
			} else {
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Employee Updated, User Check Failed: %s", user.Username, user.Email, err.Error()))
			}
			continue
		}

		if erpUser != nil {
			// ERPNext user already exists
			result.ERPUsersAlready++
			if isNewEmployee {
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Employee Created, ERPNext User Already Exists", user.Username, user.Email))
			} else {
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Already Mapped, ERPNext User Exists", user.Username, user.Email))
			}
		} else {
			// Need to create ERPNext user
			p.API.LogInfo("Creating ERPNext user for employee", "email", user.Email)

			// Generate username from email (take part before @)
			emailParts := strings.Split(user.Email, "@")
			username := emailParts[0]
			if len(username) == 0 {
				username = fmt.Sprintf("user_%s", user.Id[:8]) // Fallback to partial Mattermost ID
			}

			newERPUser := &erpnext.User{
				Email:            user.Email,
				FirstName:        user.FirstName,
				LastName:         user.LastName,
				Username:         username,
				Enabled:          1, // 1 for enabled
				RoleProfileName:  "Mặc định",
				SendWelcomeEmail: 0, // Send welcome email
			}

			_, err := p.erpNextClient.CreateUser(newERPUser)
			if err != nil {
				p.API.LogError("Failed to create ERPNext user", "email", user.Email, "error", err)
				if isNewEmployee {
					result.UserResults = append(result.UserResults,
						fmt.Sprintf("%s (%s) - Employee Created, ERPNext User Creation Failed: %s", user.Username, user.Email, err.Error()))
				} else {
					result.UserResults = append(result.UserResults,
						fmt.Sprintf("%s (%s) - Employee Updated, ERPNext User Creation Failed: %s", user.Username, user.Email, err.Error()))
				}
				continue
			}

			result.ERPUsersCreated++
			if isNewEmployee {
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Employee & ERPNext User Created", user.Username, user.Email))
			} else {
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s (%s) - Employee Updated, ERPNext User Created", user.Username, user.Email))
			}
		}
	}

	// Set total processed count
	result.TotalProcessed = result.MatchedCount + result.UpdatedCount + result.CreatedCount + result.SkippedCount

	// Create response summary
	summary := fmt.Sprintf(
		"Sync completed. Total Processed: %d, Matched: %d, Updated: %d, Created: %d, Skipped: %d, ERPNext Users Created: %d, ERPNext Users Already Exist: %d, Timed Out: %v",
		result.TotalProcessed, result.MatchedCount, result.UpdatedCount, result.CreatedCount, result.SkippedCount, result.ERPUsersCreated, result.ERPUsersAlready, result.TimedOut,
	)
	p.API.LogInfo(summary)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		p.API.LogError("Failed to encode response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SyncEmployees syncs ERPNext employees with Mattermost users - Enhanced for 500-700+ employees
func (p *Plugin) SyncEmployees(w http.ResponseWriter, r *http.Request) {
	// Log the start of function for debugging
	p.API.LogInfo("SyncEmployees function started")

	// Add timeout protection for large syncs
	startTime := time.Now()
	maxDuration := 20 * time.Minute // Increased timeout for large employee syncs

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
			"custom_chat_id",   // Field name
			"Workdone User ID", // Label
			"Employee",         // Document type
			"Data",             // Field type
			false,              // Not required
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

	// Fetch all employees from ERPNext (now with enhanced pagination)
	p.API.LogInfo("Fetching ERPNext employees with enhanced pagination")
	employees, err := p.erpNextClient.GetEmployees()
	if err != nil {
		p.API.LogError("Failed to fetch employees from ERPNext", "error", err)
		http.Error(w, fmt.Sprintf("Failed to fetch employees: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Log summary of employees fetched
	p.API.LogInfo(fmt.Sprintf("Fetched %d employees from ERPNext", len(employees)))

	// Build response data structure with enhanced tracking
	type SyncResult struct {
		MatchedCount   int      `json:"matched_count"`
		UpdatedCount   int      `json:"updated_count"`
		CreatedCount   int      `json:"created_count"`
		SkippedCount   int      `json:"skipped_count"`
		UserResults    []string `json:"user_results"`
		TotalProcessed int      `json:"total_processed"`
		TimedOut       bool     `json:"timed_out"`
		ProcessingTime string   `json:"processing_time"`
	}

	result := SyncResult{
		UserResults: []string{},
	}

	// Process each employee with enhanced progress tracking
	for i, employee := range employees {
		// Check for timeout
		if time.Since(startTime) > maxDuration {
			p.API.LogWarn("Employee sync operation reached maximum duration, stopping", "processed_employees", i)
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("TIMEOUT: Sync stopped after processing %d employees due to timeout", i))
			result.TimedOut = true
			break
		}

		// Progress logging for large syncs
		if i > 0 && i%25 == 0 {
			elapsed := time.Since(startTime)
			p.API.LogInfo(fmt.Sprintf("Employee sync progress: processed %d/%d employees (%.1f%%) in %v",
				i, len(employees), float64(i)/float64(len(employees))*100, elapsed))
		}

		// Skip if employee has no company email
		if employee.CompanyEmail == "" {
			p.API.LogDebug("Skipping employee with no company email", "employee_id", employee.Name)
			result.SkippedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s %s (%s) - Skipped (No Email)", employee.FirstName, employee.LastName, employee.Name))
			continue
		}

		// Skip if employee status is not Active
		if employee.Status != "Active" {
			p.API.LogDebug("Skipping inactive employee", "employee_id", employee.Name, "status", employee.Status)
			result.SkippedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s %s (%s) - Skipped (Inactive)", employee.FirstName, employee.LastName, employee.Name))
			continue
		}

		// Check if this employee already has a Mattermost account mapped
		if employee.CustomChatID != "" {
			// Check if the user still exists in Mattermost
			user, appErr := p.API.GetUser(employee.CustomChatID)
			if appErr == nil && user != nil && user.DeleteAt == 0 {
				// User exists and is not deleted
				result.MatchedCount++
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s %s (%s) - Already Mapped", employee.FirstName, employee.LastName, employee.CompanyEmail))
				continue
			}

			// If we get here, the mapped user doesn't exist or is deleted
			// We'll try to find a user by email or create a new one
			p.API.LogDebug("Mapped user no longer exists, will search for existing or create new",
				"employee_email", employee.CompanyEmail, "old_user_id", employee.CustomChatID)
		}

		// Try multiple approaches to find a Mattermost user with the same email
		var existingUser *model.User = nil
		var appErr *model.AppError = nil

		// First try: use GetUserByEmail which is most reliable for exact email matching
		existingUser, appErr = p.API.GetUserByEmail(employee.CompanyEmail)

		// If direct email lookup failed, try search as a fallback
		if appErr != nil || existingUser == nil {
			p.API.LogDebug("Direct email lookup failed, trying search", "email", employee.CompanyEmail, "error", appErr)

			// Try searching with broader criteria
			userSearchOpts := &model.UserSearch{
				AllowInactive: false,
				Term:          employee.CompanyEmail,
				Limit:         10, // Increased limit to catch more potential matches
			}

			userList, searchErr := p.API.SearchUsers(userSearchOpts)

			if searchErr == nil && len(userList) > 0 {
				// Look for exact email match in search results
				for _, user := range userList {
					if strings.EqualFold(user.Email, employee.CompanyEmail) && user.DeleteAt == 0 {
						existingUser = user
						p.API.LogInfo("Found user by search", "user_id", user.Id, "email", user.Email)
						break
					}
				}
			}
		}

		// Found existing user with matching email
		if existingUser != nil && existingUser.DeleteAt == 0 {
			// Update the employee's custom_chat_id in ERPNext
			updatedEmployee := &erpnext.Employee{
				Name:         employee.Name,
				CustomChatID: existingUser.Id,
			}

			_, err := p.erpNextClient.UpdateEmployee(updatedEmployee)
			if err != nil {
				p.API.LogError("Failed to update employee custom_chat_id in ERPNext",
					"employee_id", employee.Name,
					"error", err)
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s %s (%s) - Update Failed: %s", employee.FirstName, employee.LastName, employee.CompanyEmail, err.Error()))
				continue
			}

			result.UpdatedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s %s (%s) - Mapped to existing user", employee.FirstName, employee.LastName, employee.CompanyEmail))
		} else {
			// Need to create a new Mattermost user
			p.API.LogInfo("Creating new Mattermost user for ERPNext employee",
				"employee_name", fmt.Sprintf("%s %s", employee.FirstName, employee.LastName),
				"email", employee.CompanyEmail)

			// Generate username from name (slug of employee name)
			username := p.GenerateUsername(employee.FirstName, employee.LastName)

			// Check if username already exists and make it unique if needed
			for retries := 0; retries < 5; retries++ {
				_, userErr := p.API.GetUserByUsername(username)
				if userErr != nil {
					// Username doesn't exist, we can use it
					break
				}
				// Username exists, add a suffix
				username = fmt.Sprintf("%s_%d", p.GenerateUsername(employee.FirstName, employee.LastName), retries+1)
			}

			// Generate random password
			password := p.GenerateRandomPassword(12)

			// Create new user with enhanced error handling
			newUser := &model.User{
				Email:         employee.CompanyEmail,
				Username:      username,
				Password:      password,
				EmailVerified: true,
				FirstName:     employee.FirstName,
				LastName:      employee.LastName,
			}

			createdUser, appErr := p.API.CreateUser(newUser)
			if appErr != nil {
				p.API.LogError("Failed to create Mattermost user",
					"email", employee.CompanyEmail,
					"username", username,
					"error", appErr.Error())

				// Try with a different username if it's a username conflict
				if strings.Contains(appErr.Error(), "username") {
					// Generate a more unique username
					timestamp := time.Now().Unix()
					uniqueUsername := fmt.Sprintf("%s_%d", username, timestamp%10000)
					newUser.Username = uniqueUsername

					createdUser, appErr = p.API.CreateUser(newUser)
					if appErr != nil {
						result.UserResults = append(result.UserResults,
							fmt.Sprintf("%s %s (%s) - User Creation Failed (retry): %s", employee.FirstName, employee.LastName, employee.CompanyEmail, appErr.Error()))
						continue
					}
					username = uniqueUsername // Update for the response
				} else {
					result.UserResults = append(result.UserResults,
						fmt.Sprintf("%s %s (%s) - User Creation Failed: %s", employee.FirstName, employee.LastName, employee.CompanyEmail, appErr.Error()))
					continue
				}
			}

			// Update the employee's custom_chat_id in ERPNext
			updatedEmployee := &erpnext.Employee{
				Name:         employee.Name,
				CustomChatID: createdUser.Id,
			}

			_, err := p.erpNextClient.UpdateEmployee(updatedEmployee)
			if err != nil {
				p.API.LogError("Failed to update employee custom_chat_id in ERPNext after user creation",
					"employee_id", employee.Name,
					"user_id", createdUser.Id,
					"error", err)
				result.UserResults = append(result.UserResults,
					fmt.Sprintf("%s %s (%s) - User Created but Update Failed: %s", employee.FirstName, employee.LastName, employee.CompanyEmail, err.Error()))
				continue
			}

			// Attempt to send email notification with credentials
			emailSuccess := p.SendCredentialEmail(employee.CompanyEmail, username, password)

			// Add credentials to result details with email status
			emailStatus := ""
			if emailSuccess {
				emailStatus = " (Email sent)"
			} else {
				emailStatus = " (Email delivery attempted)"
			}

			result.CreatedCount++
			result.UserResults = append(result.UserResults,
				fmt.Sprintf("%s %s (%s) - New User Created%s\nUsername: %s\nPassword: %s",
					employee.FirstName, employee.LastName, employee.CompanyEmail,
					emailStatus, username, password))
		}
	}

	// Set final tracking values
	result.TotalProcessed = result.MatchedCount + result.UpdatedCount + result.CreatedCount + result.SkippedCount
	result.ProcessingTime = time.Since(startTime).String()

	// Create response summary
	summary := fmt.Sprintf(
		"Employee sync completed in %s. Total Processed: %d, Matched: %d, Updated: %d, Created: %d, Skipped: %d, Timed Out: %v",
		result.ProcessingTime, result.TotalProcessed, result.MatchedCount, result.UpdatedCount, result.CreatedCount, result.SkippedCount, result.TimedOut,
	)
	p.API.LogInfo(summary)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		p.API.LogError("Failed to encode response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
