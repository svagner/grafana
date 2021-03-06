package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/ldap"
	"github.com/grafana/grafana/pkg/services/multildap"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type LDAPMock struct {
	Results []*models.ExternalUserInfo
}

var userSearchResult *models.ExternalUserInfo
var userSearchConfig ldap.ServerConfig
var pingResult []*multildap.ServerStatus
var pingError error

func (m *LDAPMock) Ping() ([]*multildap.ServerStatus, error) {
	return pingResult, pingError
}

func (m *LDAPMock) Login(query *models.LoginUserQuery) (*models.ExternalUserInfo, error) {
	return &models.ExternalUserInfo{}, nil
}

func (m *LDAPMock) Users(logins []string) ([]*models.ExternalUserInfo, error) {
	s := []*models.ExternalUserInfo{}
	return s, nil
}

func (m *LDAPMock) User(login string) (*models.ExternalUserInfo, ldap.ServerConfig, error) {
	return userSearchResult, userSearchConfig, nil
}

//***
// GetUserFromLDAP tests
//***

func getUserFromLDAPContext(t *testing.T, requestURL string) *scenarioContext {
	t.Helper()

	sc := setupScenarioContext(requestURL)

	ldap := setting.LDAPEnabled
	setting.LDAPEnabled = true
	defer func() { setting.LDAPEnabled = ldap }()

	hs := &HTTPServer{Cfg: setting.NewCfg()}

	sc.defaultHandler = Wrap(func(c *models.ReqContext) Response {
		sc.context = c
		return hs.GetUserFromLDAP(c)
	})

	sc.m.Get("/api/admin/ldap/:username", sc.defaultHandler)

	sc.resp = httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, requestURL, nil)
	sc.req = req
	sc.exec()

	return sc
}

func TestGetUserFromLDAPApiEndpoint_UserNotFound(t *testing.T) {
	getLDAPConfig = func() (*ldap.Config, error) {
		return &ldap.Config{}, nil
	}

	newLDAP = func(_ []*ldap.ServerConfig) multildap.IMultiLDAP {
		return &LDAPMock{}
	}

	userSearchResult = nil

	sc := getUserFromLDAPContext(t, "/api/admin/ldap/user-that-does-not-exist")

	require.Equal(t, sc.resp.Code, http.StatusNotFound)
	responseString, err := getBody(sc.resp)

	assert.Nil(t, err)
	assert.Equal(t, "{\"message\":\"No user was found on the LDAP server(s)\"}", responseString)
}

func TestGetUserFromLDAPApiEndpoint_OrgNotfound(t *testing.T) {
	isAdmin := true
	userSearchResult = &models.ExternalUserInfo{
		Name:           "John Doe",
		Email:          "john.doe@example.com",
		Login:          "johndoe",
		OrgRoles:       map[int64]models.RoleType{1: models.ROLE_ADMIN, 2: models.ROLE_VIEWER},
		IsGrafanaAdmin: &isAdmin,
	}

	userSearchConfig = ldap.ServerConfig{
		Attr: ldap.AttributeMap{
			Name:     "ldap-name",
			Surname:  "ldap-surname",
			Email:    "ldap-email",
			Username: "ldap-username",
		},
		Groups: []*ldap.GroupToOrgRole{
			{
				GroupDN: "cn=admins,ou=groups,dc=grafana,dc=org",
				OrgID:   1,
				OrgRole: models.ROLE_ADMIN,
			},
			{
				GroupDN: "cn=admins,ou=groups,dc=grafana2,dc=org",
				OrgID:   2,
				OrgRole: models.ROLE_VIEWER,
			},
		},
	}

	mockOrgSearchResult := []*models.OrgDTO{
		{Id: 1, Name: "Main Org."},
	}

	bus.AddHandler("test", func(query *models.SearchOrgsQuery) error {
		query.Result = mockOrgSearchResult
		return nil
	})

	getLDAPConfig = func() (*ldap.Config, error) {
		return &ldap.Config{}, nil
	}

	newLDAP = func(_ []*ldap.ServerConfig) multildap.IMultiLDAP {
		return &LDAPMock{}
	}

	sc := getUserFromLDAPContext(t, "/api/admin/ldap/johndoe")

	require.Equal(t, sc.resp.Code, http.StatusBadRequest)

	jsonResponse, err := getJSONbody(sc.resp)
	assert.Nil(t, err)

	expected := `
	{
		"error": "Unable to find organization with ID '2'",
		"message": "An oganization was not found - Please verify your LDAP configuration"
	}
	`
	var expectedJSON interface{}
	_ = json.Unmarshal([]byte(expected), &expectedJSON)

	assert.Equal(t, expectedJSON, jsonResponse)
}

func TestGetUserFromLDAPApiEndpoint(t *testing.T) {
	isAdmin := true
	userSearchResult = &models.ExternalUserInfo{
		Name:           "John Doe",
		Email:          "john.doe@example.com",
		Login:          "johndoe",
		OrgRoles:       map[int64]models.RoleType{1: models.ROLE_ADMIN},
		IsGrafanaAdmin: &isAdmin,
	}

	userSearchConfig = ldap.ServerConfig{
		Attr: ldap.AttributeMap{
			Name:     "ldap-name",
			Surname:  "ldap-surname",
			Email:    "ldap-email",
			Username: "ldap-username",
		},
		Groups: []*ldap.GroupToOrgRole{
			{
				GroupDN: "cn=admins,ou=groups,dc=grafana,dc=org",
				OrgID:   1,
				OrgRole: models.ROLE_ADMIN,
			},
		},
	}

	mockOrgSearchResult := []*models.OrgDTO{
		{Id: 1, Name: "Main Org."},
	}

	bus.AddHandler("test", func(query *models.SearchOrgsQuery) error {
		query.Result = mockOrgSearchResult
		return nil
	})

	getLDAPConfig = func() (*ldap.Config, error) {
		return &ldap.Config{}, nil
	}

	newLDAP = func(_ []*ldap.ServerConfig) multildap.IMultiLDAP {
		return &LDAPMock{}
	}

	sc := getUserFromLDAPContext(t, "/api/admin/ldap/johndoe")

	require.Equal(t, sc.resp.Code, http.StatusOK)

	jsonResponse, err := getJSONbody(sc.resp)
	assert.Nil(t, err)

	expected := `
		{
		  "name": {
				"cfgAttrValue": "ldap-name", "ldapValue": "John"
			},
			"surname": {
				"cfgAttrValue": "ldap-surname", "ldapValue": "Doe"
			},
			"email": {
				"cfgAttrValue": "ldap-email", "ldapValue": "john.doe@example.com"
			},
			"login": {
				"cfgAttrValue": "ldap-username", "ldapValue": "johndoe"
			},
			"isGrafanaAdmin": true,
			"isDisabled": false,
			"roles": [
				{ "orgId": 1, "orgRole": "Admin", "orgName": "Main Org.", "groupDN": "cn=admins,ou=groups,dc=grafana,dc=org" }
			],
			"teams": null
		}
	`
	var expectedJSON interface{}
	_ = json.Unmarshal([]byte(expected), &expectedJSON)

	assert.Equal(t, expectedJSON, jsonResponse)
}

func TestGetUserFromLDAPApiEndpoint_WithTeamHandler(t *testing.T) {
	isAdmin := true
	userSearchResult = &models.ExternalUserInfo{
		Name:           "John Doe",
		Email:          "john.doe@example.com",
		Login:          "johndoe",
		OrgRoles:       map[int64]models.RoleType{1: models.ROLE_ADMIN},
		IsGrafanaAdmin: &isAdmin,
	}

	userSearchConfig = ldap.ServerConfig{
		Attr: ldap.AttributeMap{
			Name:     "ldap-name",
			Surname:  "ldap-surname",
			Email:    "ldap-email",
			Username: "ldap-username",
		},
		Groups: []*ldap.GroupToOrgRole{
			{
				GroupDN: "cn=admins,ou=groups,dc=grafana,dc=org",
				OrgID:   1,
				OrgRole: models.ROLE_ADMIN,
			},
		},
	}

	mockOrgSearchResult := []*models.OrgDTO{
		{Id: 1, Name: "Main Org."},
	}

	bus.AddHandler("test", func(query *models.SearchOrgsQuery) error {
		query.Result = mockOrgSearchResult
		return nil
	})

	bus.AddHandler("test", func(cmd *models.GetTeamsForLDAPGroupCommand) error {
		cmd.Result = []models.TeamOrgGroupDTO{}
		return nil
	})

	getLDAPConfig = func() (*ldap.Config, error) {
		return &ldap.Config{}, nil
	}

	newLDAP = func(_ []*ldap.ServerConfig) multildap.IMultiLDAP {
		return &LDAPMock{}
	}

	sc := getUserFromLDAPContext(t, "/api/admin/ldap/johndoe")

	require.Equal(t, sc.resp.Code, http.StatusOK)

	jsonResponse, err := getJSONbody(sc.resp)
	assert.Nil(t, err)

	expected := `
		{
		  "name": {
				"cfgAttrValue": "ldap-name", "ldapValue": "John"
			},
			"surname": {
				"cfgAttrValue": "ldap-surname", "ldapValue": "Doe"
			},
			"email": {
				"cfgAttrValue": "ldap-email", "ldapValue": "john.doe@example.com"
			},
			"login": {
				"cfgAttrValue": "ldap-username", "ldapValue": "johndoe"
			},
			"isGrafanaAdmin": true,
			"isDisabled": false,
			"roles": [
				{ "orgId": 1, "orgRole": "Admin", "orgName": "Main Org.", "groupDN": "cn=admins,ou=groups,dc=grafana,dc=org" }
			],
			"teams": []
		}
	`
	var expectedJSON interface{}
	_ = json.Unmarshal([]byte(expected), &expectedJSON)

	assert.Equal(t, expectedJSON, jsonResponse)
}

//***
// GetLDAPStatus tests
//***

func getLDAPStatusContext(t *testing.T) *scenarioContext {
	t.Helper()

	requestURL := "/api/admin/ldap/status"
	sc := setupScenarioContext(requestURL)

	ldap := setting.LDAPEnabled
	setting.LDAPEnabled = true
	defer func() { setting.LDAPEnabled = ldap }()

	hs := &HTTPServer{Cfg: setting.NewCfg()}

	sc.defaultHandler = Wrap(func(c *models.ReqContext) Response {
		sc.context = c
		return hs.GetLDAPStatus(c)
	})

	sc.m.Get("/api/admin/ldap/status", sc.defaultHandler)

	sc.resp = httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, requestURL, nil)
	sc.req = req
	sc.exec()

	return sc
}

func TestGetLDAPStatusApiEndpoint(t *testing.T) {
	pingResult = []*multildap.ServerStatus{
		{Host: "10.0.0.3", Port: 361, Available: true, Error: nil},
		{Host: "10.0.0.3", Port: 362, Available: true, Error: nil},
		{Host: "10.0.0.5", Port: 361, Available: false, Error: errors.New("something is awfully wrong")},
	}

	getLDAPConfig = func() (*ldap.Config, error) {
		return &ldap.Config{}, nil
	}

	newLDAP = func(_ []*ldap.ServerConfig) multildap.IMultiLDAP {
		return &LDAPMock{}
	}

	sc := getLDAPStatusContext(t)

	require.Equal(t, http.StatusOK, sc.resp.Code)
	jsonResponse, err := getJSONbody(sc.resp)
	assert.Nil(t, err)

	expected := `
	[
		{ "host": "10.0.0.3", "port": 361, "available": true, "error": "" },
		{ "host": "10.0.0.3", "port": 362, "available": true, "error": "" },
		{ "host": "10.0.0.5", "port": 361, "available": false, "error": "something is awfully wrong" }
	]
	`
	var expectedJSON interface{}
	_ = json.Unmarshal([]byte(expected), &expectedJSON)

	assert.Equal(t, expectedJSON, jsonResponse)
}
