package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const refA = "aaaaaaaaaaaaaaaaaaaa"
const refB = "bbbbbbbbbbbbbbbbbbbb"

type memoryCredentials struct {
	mu          sync.Mutex
	values      map[string]string
	unavailable bool
}

func (m *memoryCredentials) Set(ref, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unavailable {
		return ErrCredentialUnavailable
	}
	m.values[ref] = token
	return nil
}
func (m *memoryCredentials) Get(ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unavailable || m.values[ref] == "" {
		return "", ErrCredentialUnavailable
	}
	return m.values[ref], nil
}
func (m *memoryCredentials) Delete(ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unavailable {
		return ErrCredentialUnavailable
	}
	delete(m.values, ref)
	return nil
}

type fakeCloud struct {
	inventories map[string]CloudInventory
	failure     error
}

func (f *fakeCloud) Inventory(_ context.Context, token string) (CloudInventory, error) {
	if f.failure != nil {
		return CloudInventory{}, f.failure
	}
	value, ok := f.inventories[token]
	if !ok {
		return CloudInventory{}, cloudError("credentials_expired", "Invalid token")
	}
	return value, nil
}
func accountInventory(org, ref string) CloudInventory {
	return CloudInventory{Organizations: []CloudOrganization{{ID: org, Slug: org, Name: org}}, Projects: []CloudProject{{Ref: ref, OrganizationID: org, Name: ref, Region: "eu-west-1", Status: "ACTIVE_HEALTHY"}}, AccessState: "ready", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
}
func accountEngine(t *testing.T) (*Engine, *memoryCredentials, *fakeCloud) {
	t.Helper()
	e, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	credentials := &memoryCredentials{values: map[string]string{}}
	cloud := &fakeCloud{inventories: map[string]CloudInventory{"sbp_personal": accountInventory("personal", refA), "sbp_work": accountInventory("work", refB), "sbp_shared": accountInventory("personal", refA)}}
	e.WithAccountServices(credentials, cloud)
	return e, credentials, cloud
}
func requireAccount(t *testing.T, e *Engine, label, token string, session bool) AccountProfile {
	t.Helper()
	profile, err := e.AddAccount(context.Background(), label, token, session)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
func registerLocal(t *testing.T, e *Engine, id string) Project {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "supabase"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "supabase", "config.toml"), []byte("project_id = \""+id+"\"\n[api]\nport = 55001\n"), 0600); err != nil {
		t.Fatal(err)
	}
	project, err := e.Add(context.Background(), root, id, "")
	if err != nil {
		t.Fatal(err)
	}
	return project
}
func TestAccountsIsolateCredentialsAndInventory(t *testing.T) {
	e, credentials, _ := accountEngine(t)
	a := requireAccount(t, e, "Personal", "sbp_personal", false)
	b := requireAccount(t, e, "Work", "sbp_work", false)
	if a.CredentialRef == b.CredentialRef || credentials.values[a.CredentialRef] != "sbp_personal" {
		t.Fatal("credentials mixed")
	}
	for _, pair := range []struct{ id, ref string }{{a.ID, refA}, {b.ID, refB}} {
		inventory, err := e.CloudList(context.Background(), pair.id, true)
		if err != nil || inventory.AccountID != pair.id || inventory.Projects[0].Ref != pair.ref {
			t.Fatalf("wrong inventory: %+v %v", inventory, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(e.Store.Root, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"sbp_personal", "sbp_work"} {
		if strings.Contains(string(data), token) {
			t.Fatal("credential persisted")
		}
	}
	restarted, err := New(e.Store.Root)
	if err != nil {
		t.Fatal(err)
	}
	restarted.WithAccountServices(credentials, e.cloud)
	inventory, err := restarted.CloudList(context.Background(), a.ID, true)
	if err != nil || inventory.Projects[0].Ref != refA {
		t.Fatal("account not restored")
	}
}
func TestSharedCloudAssociationsAndDisconnectPreserveLocal(t *testing.T) {
	e, credentials, _ := accountEngine(t)
	local := registerLocal(t, e, "local")
	other := registerLocal(t, e, "worktree")
	a := requireAccount(t, e, "Personal", "sbp_personal", false)
	b := requireAccount(t, e, "Client", "sbp_shared", false)
	original, _ := os.ReadFile(filepath.Join(local.Path, "supabase", "config.toml"))
	for _, p := range []Project{local, other} {
		if _, err := e.Associate(context.Background(), p.ID, a.ID, refA); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.Associate(context.Background(), local.ID, b.ID, refA); err != nil {
		t.Fatal(err)
	}
	registry, _ := e.Registry()
	if len(registry.Associations) != 2 || len(registry.Projects) != 2 {
		t.Fatal("duplicated local registration or association")
	}
	if err := e.RemoveAccount(a.ID); err != nil {
		t.Fatal(err)
	}
	registry, _ = e.Registry()
	if len(registry.Associations) != 2 || len(registry.Projects) != 2 || len(registry.Accounts) != 1 {
		t.Fatal("disconnect removed local state")
	}
	if _, ok := credentials.values[a.CredentialRef]; ok {
		t.Fatal("credential retained")
	}
	now, _ := os.ReadFile(filepath.Join(local.Path, "supabase", "config.toml"))
	if string(original) != string(now) {
		t.Fatal("config mutated")
	}
	if err := e.Unassociate(local.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Remove(other.ID); err != nil {
		t.Fatal(err)
	}
	registry, _ = e.Registry()
	if len(registry.Associations) != 0 {
		t.Fatal("dangling removed-local association")
	}
}
func TestSessionOnlyAndUnavailableCredentials(t *testing.T) {
	e, credentials, _ := accountEngine(t)
	credentials.unavailable = true
	if _, err := e.AddAccount(context.Background(), "Saved", "sbp_personal", false); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatal(err)
	}
	session := requireAccount(t, e, "Session", "sbp_personal", true)
	registry, err := e.Store.Read()
	if err != nil || len(registry.Accounts) != 0 {
		t.Fatal("session persisted")
	}
	profiles, _ := e.Accounts()
	if len(profiles) != 1 || !profiles[0].SessionOnly {
		t.Fatal("session missing")
	}
	inventory, err := e.CloudList(context.Background(), session.ID, true)
	if err != nil || inventory.AccessState != "ready" {
		t.Fatal(err)
	}
	restarted, _ := New(e.Store.Root)
	profiles, _ = restarted.Accounts()
	if len(profiles) != 0 {
		t.Fatal("session restored")
	}
	if err := e.RemoveAccount(session.ID); err != nil {
		t.Fatal(err)
	}
}
func TestCloudFailuresRetainCacheAndPreventAssociation(t *testing.T) {
	for _, state := range []string{"credentials_expired", "permission_denied", "rate_limited", "unavailable", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			e, _, cloud := accountEngine(t)
			a := requireAccount(t, e, "Personal", "sbp_personal", false)
			local := registerLocal(t, e, "local")
			cloud.failure = cloudError(state, "Actionable failure")
			inventory, err := e.CloudList(context.Background(), a.ID, true)
			if err != nil || !inventory.Stale || inventory.AccessState != state || inventory.Projects[0].Ref != refA {
				t.Fatalf("cache lost: %+v %v", inventory, err)
			}
			if _, err := e.Associate(context.Background(), local.ID, a.ID, refA); err == nil {
				t.Fatal("associated with failed access")
			}
			registry, _ := e.Registry()
			if len(registry.Associations) != 0 {
				t.Fatal("wrote failed association")
			}
		})
	}
}
func TestAssociationSuggestionsRequireExactReferenceAndConfirmation(t *testing.T) {
	e, _, _ := accountEngine(t)
	local := registerLocal(t, e, "local")
	a := requireAccount(t, e, "Personal", "sbp_personal", false)
	b := requireAccount(t, e, "Shared", "sbp_shared", false)
	path := filepath.Join(local.Path, "supabase", ".temp")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "project-ref"), []byte(refA), 0600); err != nil {
		t.Fatal(err)
	}
	suggestions, err := e.AssociationSuggestions(local.ID)
	if err != nil || len(suggestions) != 1 || len(suggestions[0].AccountIDs) != 2 {
		t.Fatalf("%+v %v", suggestions, err)
	}
	registry, _ := e.Registry()
	if len(registry.Associations) != 0 {
		t.Fatal("suggestion applied automatically")
	}
	if _, err := e.Associate(context.Background(), local.ID, a.ID, refB); err == nil {
		t.Fatal("inaccessible reference accepted")
	}
	if _, err := e.DashboardURL(context.Background(), b.ID, "https://evil.test"); err == nil {
		t.Fatal("untrusted dashboard URL")
	}
	if err := os.WriteFile(filepath.Join(path, "project-ref"), []byte("same-name"), 0600); err != nil {
		t.Fatal(err)
	}
	suggestions, err = e.AssociationSuggestions(local.ID)
	if err != nil || len(suggestions) != 0 {
		t.Fatal("matched a project name")
	}
}
func TestAccountRegistrationRejectsInvalidAccessAndDuplicateLabels(t *testing.T) {
	e, _, cloud := accountEngine(t)
	for _, token := range []string{"", "sbp_bad\nheader", "sb_secret_key", "sbp_invalid"} {
		if _, err := e.AddAccount(context.Background(), "Profile", token, false); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	requireAccount(t, e, "Personal", "sbp_personal", false)
	if _, err := e.AddAccount(context.Background(), "Personal", "sbp_work", false); err == nil {
		t.Fatal("duplicate label")
	}
	cloud.failure = cloudError("permission_denied", "Missing read permissions")
	if _, err := e.AddAccount(context.Background(), "Denied", "sbp_work", false); err == nil {
		t.Fatal("unvalidated account persisted")
	}
	profiles, _ := e.Accounts()
	if len(profiles) != 1 {
		t.Fatal("failed profiles saved")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func metadataResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestManagementInventoryPaginationAndNoSecretFields(t *testing.T) {
	calls := []string{}
	client := NewManagementClient()
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer sbp_private" || r.URL.Host != "api.supabase.com" {
			t.Fatal("unsafe request")
		}
		calls = append(calls, r.URL.RequestURI())
		if r.URL.Path == "/v1/organizations" {
			return metadataResponse(200, `[{"id":"org","slug":"org","name":"Org"}]`), nil
		}
		if r.URL.Query().Get("offset") == "0" {
			return metadataResponse(200, fmt.Sprintf(`{"projects":[{"ref":%q,"name":"sbp_private","region":"eu","status":"ACTIVE_HEALTHY","database_password":"sbp_private"}],"pagination":{"count":2,"limit":1,"offset":0}}`, refA)), nil
		}
		return metadataResponse(200, fmt.Sprintf(`{"projects":[{"ref":%q,"name":"B"}],"pagination":{"count":2,"limit":1,"offset":1}}`, refB)), nil
	})
	inventory, err := client.Inventory(context.Background(), "sbp_private")
	if err != nil || len(inventory.Projects) != 2 || len(calls) != 3 {
		t.Fatalf("pagination: %+v %v", inventory, err)
	}
	data, _ := json.Marshal(inventory)
	if strings.Contains(string(data), "sbp_private") || strings.Contains(string(data), "database_password") {
		t.Fatal("secret metadata leaked")
	}
}
func TestManagementErrorsDoNotEchoResponseOrToken(t *testing.T) {
	for _, code := range []int{401, 403, 429, 500, 302} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			client := NewManagementClient()
			client.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) { return metadataResponse(code, "sbp_private"), nil })
			_, err := client.Inventory(context.Background(), "sbp_private")
			if err == nil || strings.Contains(err.Error(), "sbp_private") {
				t.Fatal("unsafe API error")
			}
		})
	}
	client := NewManagementClient()
	client.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) { return nil, errors.New("sbp_private transport failure") })
	_, err := client.Inventory(context.Background(), "sbp_private")
	if err == nil || strings.Contains(err.Error(), "sbp_private") {
		t.Fatal("unsafe transport error")
	}
}
func TestAccountDispatchAndRegistryCompatibility(t *testing.T) {
	e, _, _ := accountEngine(t)
	if err := os.WriteFile(filepath.Join(e.Store.Root, "registry.json"), []byte(`{"projects":[],"settings":{"supabase_cli":"supabase","docker_cli":"docker"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := e.Handle(context.Background(), Request{Action: "account_add", Name: "Personal", Token: "sbp_personal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := result.(AccountProfile)
	result, err = e.Handle(context.Background(), Request{Action: "cloud_list", Account: profile.ID}, nil)
	if err != nil || result.(CloudInventory).AccountID != profile.ID {
		t.Fatal("desktop contract mismatch")
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "sbp_personal") {
		t.Fatal("token in JSON")
	}
	registry, _ := e.Registry()
	if registry.Projects == nil || registry.Associations == nil {
		t.Fatal("legacy registry incompatible")
	}
}
func TestCredentialRemovalFailureKeepsRecoverableProfile(t *testing.T) {
	e, credentials, _ := accountEngine(t)
	a := requireAccount(t, e, "Personal", "sbp_personal", false)
	credentials.unavailable = true
	if err := e.RemoveAccount(a.ID); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatal(err)
	}
	profiles, _ := e.Accounts()
	if len(profiles) != 1 {
		t.Fatal("lost credential reference")
	}
}
func TestConcurrentAccountCommandsUseSharedLock(t *testing.T) {
	e, _, _ := accountEngine(t)
	lock, err := e.Store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	if _, err := e.AddAccount(context.Background(), "Personal", "sbp_personal", false); err == nil {
		t.Fatal("mutation ignored shared lock")
	}
	profiles, _ := e.Accounts()
	if len(profiles) != 0 {
		t.Fatal("lock failure persisted account")
	}
}

func TestManagementEmptyAndInvalidInventory(t *testing.T) {
	for _, test := range []struct {
		name, organizations, projects string
		wantError                     bool
	}{
		{"empty", "[]", "", false},
		{"empty organization", "[{\"id\":\"org\",\"slug\":\"org\",\"name\":\"Org\"}]", "{\"projects\":[],\"pagination\":{\"count\":0}}", false},
		{"null organizations", "null", "", true},
		{"missing project list", "[{\"id\":\"org\",\"slug\":\"org\"}]", "{}", true},
		{"incomplete page", "[{\"id\":\"org\",\"slug\":\"org\"}]", "{\"projects\":[],\"pagination\":{\"count\":5}}", true},
		{"unexpected reference", "[{\"id\":\"org\",\"slug\":\"org\"}]", "{\"projects\":[{\"ref\":\"bad/ref\"}]}", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewManagementClient()
			client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1/organizations" {
					return metadataResponse(200, test.organizations), nil
				}
				return metadataResponse(200, test.projects), nil
			})
			inventory, err := client.Inventory(context.Background(), "sbp_private")
			if (err != nil) != test.wantError {
				t.Fatal(err)
			}
			if !test.wantError && len(inventory.Projects) != 0 {
				t.Fatal("empty inventory populated")
			}
		})
	}
}
func TestManagementCancellationAndRepeatedPage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewManagementClient()
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	_, err := client.Inventory(ctx, "sbp_private")
	state, _ := accessFailure(err)
	if state != "cancelled" {
		t.Fatal(err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/organizations" {
			return metadataResponse(200, `[{"id":"org","slug":"org"}]`), nil
		}
		return metadataResponse(200, fmt.Sprintf(`{"projects":[{"ref":%q}],"pagination":{"count":2}}`, refA)), nil
	})
	if _, err := client.Inventory(context.Background(), "sbp_private"); err == nil {
		t.Fatal("repeated page accepted")
	}
}
func TestCloudUnavailableCredentialsAndCacheAge(t *testing.T) {
	e, credentials, _ := accountEngine(t)
	a := requireAccount(t, e, "Personal", "sbp_personal", false)
	credentials.unavailable = true
	inventory, err := e.CloudList(context.Background(), a.ID, true)
	if err != nil || !inventory.Stale || inventory.AccessState != "credentials_unavailable" || len(inventory.Projects) != 1 {
		t.Fatalf("%+v %v", inventory, err)
	}
	aged := ageInventory(CloudInventory{UpdatedAt: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)})
	if !aged.Stale {
		t.Fatal("old cache reported fresh")
	}
}
