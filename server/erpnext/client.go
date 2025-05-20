// erpnext/client.go - Fixed version without Logger references

package erpnext

import (
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

// Employee represents an employee in ERPNext
type Employee struct {
	Name         string `json:"name"` // This is the employee ID
	EmployeeName string `json:"employee_name,omitempty"`
	CompanyEmail string `json:"company_email,omitempty"`
}

// EmployeeResponse represents the response from ERPNext API when fetching employees
type EmployeeResponse struct {
	Data []Employee `json:"data"`
}

// NewClient creates a new ERPNext client
func NewClient(url, apiKey, apiSecret string) *Client {
	return &Client{
		URL:       url,
		APIKey:    apiKey,
		APISecret: apiSecret,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetEmployees fetches all employees from ERPNext
func (c *Client) GetEmployees() ([]Employee, error) {
	url := fmt.Sprintf("%s/api/resource/Employee", c.URL)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	// Set authorization header with token format: "token api_key:api_secret"
	authToken := fmt.Sprintf("token %s:%s", c.APIKey, c.APISecret)
	req.Header.Set("Authorization", authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ERPNext API returned non-OK status code %d: %s", resp.StatusCode, string(body))
	}

	var employeeResp EmployeeResponse
	if err := json.NewDecoder(resp.Body).Decode(&employeeResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return employeeResp.Data, nil
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
	reqURL.RawQuery = query.Encode()

	// Print the request URL for debugging (this would normally go to logs)
	fmt.Printf("Making request to: %s\n", reqURL.String())

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
	fmt.Printf("Response status: %d\n", resp.StatusCode)
	fmt.Printf("Response body: %s\n", string(body))

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
