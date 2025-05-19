package erpnext

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Name         string `json:"name"`
	EmployeeName string `json:"employee_name"`
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
