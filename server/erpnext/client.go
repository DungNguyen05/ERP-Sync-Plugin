// erpnext/client.go - Enhanced version for large employee syncing

package erpnext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

// Client represents a client for interacting with ERPNext API
type Client struct {
	URL        string
	APIKey     string
	APISecret  string
	HTTPClient *http.Client
}

type CustomFieldResponse struct {
	Data []CustomField `json:"data"`
}

// CustomField represents a custom field in ERPNext
type CustomField struct {
	Name              string `json:"name"`
	FieldName         string `json:"fieldname"`
	Label             string `json:"label"`
	DocType           string `json:"dt"`
	FieldType         string `json:"fieldtype"`
	Mandatory         int    `json:"reqd"`
	Translatable      int    `json:"translatable"`
	Unique            int    `json:"unique"`
	NoQuickEntry      int    `json:"no_copy"`
	AllowInQuickEntry int    `json:"allow_in_quick_entry"`
	ReadOnly          int    `json:"read_only"`
	HideDisplay       int    `json:"hide_display"`
	Bold              int    `json:"bold"`
}

// Employee represents an employee in ERPNext
type Employee struct {
	Name          string `json:"name,omitempty"` // This is the employee ID
	CompanyEmail  string `json:"company_email,omitempty"`
	FirstName     string `json:"first_name,omitempty"`
	LastName      string `json:"last_name,omitempty"`
	Gender        string `json:"gender,omitempty"`
	DateOfBirth   string `json:"date_of_birth,omitempty"`
	DateOfJoining string `json:"date_of_joining,omitempty"`
	Status        string `json:"status,omitempty"`
	CustomChatID  string `json:"custom_chat_id,omitempty"` // New field for Mattermost ID
}

// EmployeeResponse represents the response from ERPNext API when fetching employees
type EmployeeResponse struct {
	Data []Employee `json:"data"`
}

// RoleProfile represents a role profile in ERPNext
type RoleProfile struct {
	Name            string `json:"name,omitempty"`
	RoleProfileName string `json:"role_profile,omitempty"`
}

// RoleProfileResponse represents the response from ERPNext API when fetching role profiles
type RoleProfileResponse struct {
	Data []RoleProfile `json:"data"`
}

// User represents a user in ERPNext
type User struct {
	Name             string `json:"name,omitempty"` // This is the user ID
	Email            string `json:"email,omitempty"`
	FirstName        string `json:"first_name,omitempty"`
	LastName         string `json:"last_name,omitempty"`
	Username         string `json:"username,omitempty"`
	Enabled          int    `json:"enabled,omitempty"` // 1 for enabled, 0 for disabled
	RoleProfileName  string `json:"role_profile_name,omitempty"`
	SendWelcomeEmail int    `json:"send_welcome_email,omitempty"`
}

// UserResponse represents the response from ERPNext API when fetching users
type UserResponse struct {
	Data []User `json:"data"`
}

// NewClient creates a new ERPNext client
func NewClient(url, apiKey, apiSecret string) *Client {
	return &Client{
		URL:       url,
		APIKey:    apiKey,
		APISecret: apiSecret,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second, // Increased timeout for large operations
		},
	}
}

// GetEmployees fetches all employees from ERPNext with enhanced pagination
func (c *Client) GetEmployees() ([]Employee, error) {
	allEmployees := []Employee{}
	pageSize := 200 // Increased page size for better performance
	startIdx := 0
	maxPages := 20 // Safety limit: 20 pages * 200 per page = 4000 employees max

	fmt.Printf("Starting to fetch employees from ERPNext...\n")

	for page := 0; page < maxPages; page++ {
		// Build URL with paging parameters and fields we need
		baseURL := fmt.Sprintf("%s/api/resource/Employee", c.URL)
		reqURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse URL")
		}

		// Add pagination parameters and specify fields to include
		query := reqURL.Query()
		query.Add("limit_start", fmt.Sprintf("%d", startIdx))
		query.Add("limit_page_length", fmt.Sprintf("%d", pageSize))
		query.Add("fields", `["name", "company_email", "first_name", "last_name", "gender", "date_of_birth", "date_of_joining", "status", "custom_chat_id"]`)

		// Add filter to get only active employees to improve performance
		query.Add("filters", `[["status", "=", "Active"]]`)

		reqURL.RawQuery = query.Encode()

		fmt.Printf("Fetching page %d (start: %d, limit: %d)...\n", page+1, startIdx, pageSize)

		// Create the request
		req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create request")
		}

		// Set authorization header with token format: "token api_key:api_secret"
		authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
		req.Header.Set("Authorization", authToken)
		req.Header.Set("Content-Type", "application/json")

		// Execute the request
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, errors.Wrap(err, "failed to execute request")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("ERPNext API returned non-OK status code %d: %s", resp.StatusCode, string(body))
		}

		// Parse the response
		var employeeResp EmployeeResponse
		if err := json.NewDecoder(resp.Body).Decode(&employeeResp); err != nil {
			return nil, errors.Wrap(err, "failed to decode response")
		}

		// Add the fetched employees to our result array
		allEmployees = append(allEmployees, employeeResp.Data...)

		fmt.Printf("Page %d: fetched %d employees (total so far: %d)\n",
			page+1, len(employeeResp.Data), len(allEmployees))

		// If we got fewer records than the page size, we've reached the end
		if len(employeeResp.Data) < pageSize {
			fmt.Printf("Reached end of data at page %d\n", page+1)
			break
		}

		// Update start index for the next page
		startIdx += pageSize
	}

	fmt.Printf("Completed fetching employees: %d total employees found\n", len(allEmployees))
	return allEmployees, nil
}

// GetEmployeeByEmail finds an employee by company email
func (c *Client) GetEmployeeByEmail(email string) (*Employee, error) {
	// Create the filter parameter - try a more flexible search
	filterParam := fmt.Sprintf(`[["company_email","=","%s"]]`, email)

	// Build the URL with properly encoded query parameters
	baseURL := fmt.Sprintf("%s/api/resource/Employee", c.URL)
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL")
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name", "company_email", "first_name", "last_name", "gender", "date_of_birth", "date_of_joining", "status", "custom_chat_id"]`)
	reqURL.RawQuery = query.Encode()

	// Print the request URL for debugging (this would normally go to logs)
	fmt.Printf("Making employee search request to: %s\n", reqURL.String())

	// Now create the request with the properly encoded URL
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	// Set authorization header
	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	// Read the response body
	body, _ := io.ReadAll(resp.Body)

	// Print response for debugging
	fmt.Printf("Employee search response status: %d\n", resp.StatusCode)
	fmt.Printf("Employee search response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERPNext API returned non-OK status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var employeeResp EmployeeResponse
	if err := json.Unmarshal(body, &employeeResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode response: "+string(body))
	}

	// Print found employees for debugging
	fmt.Printf("Found %d employees with email similar to %s\n", len(employeeResp.Data), email)

	// If no employee found with that email
	if len(employeeResp.Data) == 0 {
		return nil, nil
	}

	// Return the first matching employee
	return &employeeResp.Data[0], nil
}

// CreateEmployee creates a new employee in ERPNext
func (c *Client) CreateEmployee(employee *Employee) (*Employee, error) {
	url := fmt.Sprintf("%s/api/resource/Employee", c.URL)

	// The ERPNext API expects data in a specific format with a "doc" wrapper
	requestBody := map[string]interface{}{
		"doctype":         "Employee",
		"company_email":   employee.CompanyEmail,
		"first_name":      employee.FirstName,
		"last_name":       employee.LastName,
		"gender":          employee.Gender,
		"date_of_birth":   employee.DateOfBirth,
		"date_of_joining": employee.DateOfJoining,
		"status":          employee.Status,
		"custom_chat_id":  employee.CustomChatID,
	}

	// Convert to JSON
	bodyData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal employee data")
	}

	// Print the request body for debugging
	fmt.Printf("Create employee request body: %s\n", string(bodyData))

	// Create request
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyData))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	// Set headers
	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	// Read response body for logging and error handling
	body, _ := io.ReadAll(resp.Body)

	// Log the response for debugging
	fmt.Printf("Create employee response status: %d\n", resp.StatusCode)
	fmt.Printf("Create employee response body: %s\n", string(body))

	// Handle response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ERPNext API returned status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response to get the created employee
	var respData struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, errors.Wrap(err, "failed to decode response: "+string(body))
	}

	// Return a new Employee with just the ID since that's what we need
	return &Employee{
		Name: respData.Data.Name,
	}, nil
}

// UpdateEmployee updates an existing employee in ERPNext
func (c *Client) UpdateEmployee(employee *Employee) (*Employee, error) {
	// Create URL for updating specific employee by name (ID)
	url := fmt.Sprintf("%s/api/resource/Employee/%s", c.URL, employee.Name)

	// In ERPNext, when updating we only need to include the fields we want to change
	requestBody := map[string]interface{}{
		"custom_chat_id": employee.CustomChatID,
	}

	// Convert to JSON
	bodyData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal employee update data")
	}

	// Print the request body for debugging
	fmt.Printf("Update employee request to: %s\n", url)
	fmt.Printf("Update employee request body: %s\n", string(bodyData))

	// Create PUT request for updating
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(bodyData))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create update request")
	}

	// Set headers
	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute update request")
	}
	defer resp.Body.Close()

	// Read response body for logging and error handling
	body, _ := io.ReadAll(resp.Body)

	// Log the response for debugging
	fmt.Printf("Update employee response status: %d\n", resp.StatusCode)
	fmt.Printf("Update employee response body: %s\n", string(body))

	// Handle response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("ERPNext API returned status code %d when updating employee: %s",
			resp.StatusCode, string(body))
	}

	// For update operations, ERPNext might return different formats than create
	// In many cases, it just returns a success message without the full record
	// We'll just return the original employee object since we don't need the response data
	return employee, nil
}

// CheckCustomFieldExists checks if a custom field exists for a specific DocType
func (c *Client) CheckCustomFieldExists(fieldName, docType string) (bool, error) {
	// Build URL with filters for the custom field
	baseURL := fmt.Sprintf("%s/api/resource/Custom Field", c.URL)
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse URL")
	}

	// Create the filter to find the exact field by name and document type
	filterParam := fmt.Sprintf(`[["fieldname","=","%s"],["dt","=","%s"]]`, fieldName, docType)

	// Add query parameters
	query := reqURL.Query()
	query.Add("filters", filterParam)
	reqURL.RawQuery = query.Encode()

	// Create the request
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return false, errors.Wrap(err, "failed to create request")
	}

	// Set authorization header
	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	// Read the response body
	body, _ := io.ReadAll(resp.Body)

	// Print response for debugging
	fmt.Printf("Custom field check response status: %d\n", resp.StatusCode)
	fmt.Printf("Custom field check response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("ERPNext API returned non-OK status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var customFieldResp CustomFieldResponse
	if err := json.Unmarshal(body, &customFieldResp); err != nil {
		return false, errors.Wrap(err, "failed to decode response: "+string(body))
	}

	// Field exists if we found at least one result
	return len(customFieldResp.Data) > 0, nil
}

// CreateCustomField creates a new custom field in ERPNext
func (c *Client) CreateCustomField(fieldName, label, docType, fieldType string, required bool) error {
	url := fmt.Sprintf("%s/api/resource/Custom Field", c.URL)

	// Convert boolean to integer (0 or 1)
	reqd := 0
	if required {
		reqd = 1
	}

	// The ERPNext API expects data in a specific format
	requestBody := map[string]interface{}{
		"doctype":              "Custom Field",
		"dt":                   docType,         // Document Type (e.g., "Employee")
		"fieldname":            fieldName,       // Field name (e.g., "custom_chat_id")
		"label":                label,           // Label (e.g., "Workdone User ID")
		"fieldtype":            fieldType,       // Field type (e.g., "Data")
		"insert_after":         "employee_name", // Insert after employee name for visibility
		"reqd":                 reqd,            // Is it required? (0 for not mandatory)
		"in_list_view":         0,               // Show in list view (1 for yes) - THIS IS THE KEY SETTING
		"in_standard_filter":   1,               // Include in standard filters
		"in_global_search":     1,               // Include in global search
		"allow_in_quick_entry": 1,               // Allow in quick entry
		"translatable":         0,               // Is it translatable? (0 or 1)
		"unique":               0,               // Is it unique? (0 or 1)
		"no_copy":              0,               // Exclude from copying? (0 or 1)
		"read_only":            0,               // Is it read-only? (0 or 1)
		"hide_display":         0,               // Hide in grid view? (0 or 1)
	}

	// Convert to JSON
	bodyData, err := json.Marshal(requestBody)
	if err != nil {
		return errors.Wrap(err, "failed to marshal custom field data")
	}

	// Print the request body for debugging
	fmt.Printf("Create custom field request body: %s\n", string(bodyData))

	// Create request
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyData))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	// Set headers
	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	// Read response body for logging and error handling
	body, _ := io.ReadAll(resp.Body)

	// Log the response for debugging
	fmt.Printf("Create custom field response status: %d\n", resp.StatusCode)
	fmt.Printf("Create custom field response body: %s\n", string(body))

	// Handle response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("ERPNext API returned status code %d when creating custom field: %s",
			resp.StatusCode, string(body))
	}

	return nil
}

// CheckRoleProfileExists checks if a role profile exists
func (c *Client) CheckRoleProfileExists(roleProfileName string) (bool, error) {
	baseURL := fmt.Sprintf("%s/api/resource/Role Profile", c.URL)
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse URL")
	}

	// Create filter to find role profile by name
	filterParam := fmt.Sprintf(`[["role_profile","=","%s"]]`, roleProfileName)

	query := reqURL.Query()
	query.Add("filters", filterParam)
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return false, errors.Wrap(err, "failed to create request")
	}

	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Role profile check response status: %d\n", resp.StatusCode)
	fmt.Printf("Role profile check response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("ERPNext API returned non-OK status code %d: %s", resp.StatusCode, string(body))
	}

	var roleProfileResp RoleProfileResponse
	if err := json.Unmarshal(body, &roleProfileResp); err != nil {
		return false, errors.Wrap(err, "failed to decode response: "+string(body))
	}

	return len(roleProfileResp.Data) > 0, nil
}

// CreateRoleProfile creates a new role profile
func (c *Client) CreateRoleProfile(roleProfileName string) error {
	url := fmt.Sprintf("%s/api/resource/Role Profile", c.URL)

	requestBody := map[string]interface{}{
		"doctype":      "Role Profile",
		"role_profile": roleProfileName,
		// Add comprehensive roles for full permissions
		"roles": []map[string]interface{}{
			{"role": "System Manager"},
			{"role": "Administrator"},
			{"role": "Employee"},
			{"role": "Employee Self Service"},
			{"role": "HR Manager"},
			{"role": "HR User"},
			{"role": "Accounts Manager"},
			{"role": "Accounts User"},
			{"role": "Sales Manager"},
			{"role": "Sales User"},
			{"role": "Purchase Manager"},
			{"role": "Purchase User"},
			{"role": "Stock Manager"},
			{"role": "Stock User"},
			{"role": "Manufacturing Manager"},
			{"role": "Manufacturing User"},
			{"role": "Projects Manager"},
			{"role": "Projects User"},
			{"role": "Website Manager"},
			{"role": "Desk User"},
			{"role": "All"},
		},
	}

	bodyData, err := json.Marshal(requestBody)
	if err != nil {
		return errors.Wrap(err, "failed to marshal role profile data")
	}

	fmt.Printf("Create role profile request body: %s\n", string(bodyData))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyData))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Create role profile response status: %d\n", resp.StatusCode)
	fmt.Printf("Create role profile response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("ERPNext API returned status code %d when creating role profile: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetUserByEmail finds a user by email
func (c *Client) GetUserByEmail(email string) (*User, error) {
	baseURL := fmt.Sprintf("%s/api/resource/User", c.URL)
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL")
	}

	filterParam := fmt.Sprintf(`[["email","=","%s"]]`, email)

	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name", "email", "first_name", "last_name", "username", "enabled", "role_profile_name"]`)
	reqURL.RawQuery = query.Encode()

	fmt.Printf("Making user search request to: %s\n", reqURL.String())

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("User search response status: %d\n", resp.StatusCode)
	fmt.Printf("User search response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERPNext API returned non-OK status code %d: %s", resp.StatusCode, string(body))
	}

	var userResp UserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode response: "+string(body))
	}

	fmt.Printf("Found %d users with email %s\n", len(userResp.Data), email)

	if len(userResp.Data) == 0 {
		return nil, nil
	}

	return &userResp.Data[0], nil
}

// CreateUser creates a new user in ERPNext
func (c *Client) CreateUser(user *User) (*User, error) {
	url := fmt.Sprintf("%s/api/resource/User", c.URL)

	requestBody := map[string]interface{}{
		"doctype":            "User",
		"email":              user.Email,
		"first_name":         user.FirstName,
		"last_name":          user.LastName,
		"username":           user.Username,
		"enabled":            user.Enabled,
		"role_profile_name":  user.RoleProfileName,
		"send_welcome_email": user.SendWelcomeEmail,
	}

	bodyData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal user data")
	}

	fmt.Printf("Create user request body: %s\n", string(bodyData))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyData))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Create user response status: %d\n", resp.StatusCode)
	fmt.Printf("Create user response body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ERPNext API returned status code %d when creating user: %s", resp.StatusCode, string(body))
	}

	var respData struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, errors.Wrap(err, "failed to decode response: "+string(body))
	}

	return &User{
		Name: respData.Data.Name,
	}, nil
}
