package command

import (
	"testing"

	"github.com/mattermost/mattermost-plugin-starter-template/server/erpnext"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type env struct {
	client *pluginapi.Client
	api    *plugintest.API
}

// MockERPNextClient is a mock implementation of the ERPNext client for testing
type MockERPNextClient struct {
	mock.Mock
}

func (m *MockERPNextClient) GetEmployees() ([]erpnext.Employee, error) {
	args := m.Called()
	return args.Get(0).([]erpnext.Employee), args.Error(1)
}

func setupTest() *env {
	api := &plugintest.API{}
	driver := &plugintest.Driver{}
	client := pluginapi.NewClient(api, driver)

	return &env{
		client: client,
		api:    api,
	}
}

func TestHelloCommand(t *testing.T) {
	assert := assert.New(t)
	env := setupTest()

	env.api.On("RegisterCommand", mock.Anything).Return(nil)
	cmdHandler := NewCommandHandler(env.client)

	args := &model.CommandArgs{
		Command: "/hello world",
	}
	response, err := cmdHandler.Handle(args)
	assert.Nil(err)
	assert.Equal("Hello, world", response.Text)
}

func TestEmployeeCommand(t *testing.T) {
	assert := assert.New(t)
	env := setupTest()

	env.api.On("RegisterCommand", mock.Anything).Return(nil)
	cmdHandler := NewCommandHandler(env.client)

	// Create a mock ERPNext client
	mockERPClient := new(MockERPNextClient)

	// Set up the mock to return some test employees
	testEmployees := []erpnext.Employee{
		{Name: "EMP001", EmployeeName: "John Doe"},
		{Name: "EMP002", EmployeeName: "Jane Smith"},
	}
	mockERPClient.On("GetEmployees").Return(testEmployees, nil)

	// Set the mock client in the command handler
	cmdHandler.SetERPNextClient(mockERPClient)

	// Create the command args
	args := &model.CommandArgs{
		Command: "/employee",
	}

	// Execute the command
	response, err := cmdHandler.Handle(args)

	// Verify the results
	assert.Nil(err)
	assert.Equal("There are 2 employees in ERPNext.", response.Text)
	assert.Equal(model.CommandResponseTypeInChannel, response.ResponseType)

	// Verify that the mock was called
	mockERPClient.AssertExpectations(t)
}

func TestEmployeeCommandNoEmployees(t *testing.T) {
	assert := assert.New(t)
	env := setupTest()

	env.api.On("RegisterCommand", mock.Anything).Return(nil)
	cmdHandler := NewCommandHandler(env.client)

	// Create a mock ERPNext client
	mockERPClient := new(MockERPNextClient)

	// Set up the mock to return no employees
	var emptyEmployees []erpnext.Employee
	mockERPClient.On("GetEmployees").Return(emptyEmployees, nil)

	// Set the mock client in the command handler
	cmdHandler.SetERPNextClient(mockERPClient)

	// Create the command args
	args := &model.CommandArgs{
		Command: "/employee",
	}

	// Execute the command
	response, err := cmdHandler.Handle(args)

	// Verify the results
	assert.Nil(err)
	assert.Equal("No employees found in ERPNext.", response.Text)
	assert.Equal(model.CommandResponseTypeInChannel, response.ResponseType)

	// Verify that the mock was called
	mockERPClient.AssertExpectations(t)
}

func TestEmployeeCommandNoClient(t *testing.T) {
	assert := assert.New(t)
	env := setupTest()

	env.api.On("RegisterCommand", mock.Anything).Return(nil)
	cmdHandler := NewCommandHandler(env.client)

	// Do not set an ERPNext client

	// Create the command args
	args := &model.CommandArgs{
		Command: "/employee",
	}

	// Execute the command
	response, err := cmdHandler.Handle(args)

	// Verify the results
	assert.Nil(err)
	assert.Contains(response.Text, "ERPNext client is not configured properly")
	assert.Equal(model.CommandResponseTypeEphemeral, response.ResponseType)
}
