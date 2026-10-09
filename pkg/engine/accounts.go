package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type AccountProfile struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	CredentialRef string         `json:"credential_ref,omitempty"`
	SessionOnly   bool           `json:"session_only"`
	Inventory     CloudInventory `json:"inventory"`
}
type LocalProjectAssociation struct {
	LocalProjectID string `json:"local_project_id"`
	CloudRef       string `json:"cloud_ref"`
}
type AssociationSuggestion struct {
	LocalProjectID string   `json:"local_project_id"`
	CloudRef       string   `json:"cloud_ref"`
	AccountIDs     []string `json:"account_ids"`
}
type sessionAccount struct {
	Profile AccountProfile
	Token   string
}

// WithAccountServices supplies cloud and credential adapters for integration tests.
func (e *Engine) WithAccountServices(credentials CredentialStore, cloud CloudReader) {
	e.accountMu.Lock()
	defer e.accountMu.Unlock()
	e.credentials = credentials
	e.cloud = cloud
}
func (e *Engine) profiles(registry Registry) []AccountProfile {
	profiles := slices.Clone(registry.Accounts)
	for _, session := range e.sessions {
		profiles = append(profiles, session.Profile)
	}
	return profiles
}
func findAccount(profiles []AccountProfile, id string) (AccountProfile, error) {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	var matches []AccountProfile
	for _, profile := range profiles {
		if profile.Label == id {
			matches = append(matches, profile)
		}
	}
	if len(matches) != 1 {
		return AccountProfile{}, errors.New("account not found or label is ambiguous; use an ID from account list")
	}
	return matches[0], nil
}
func (e *Engine) Accounts() ([]AccountProfile, error) {
	e.accountMu.Lock()
	defer e.accountMu.Unlock()
	registry, err := e.Store.Read()
	if err != nil {
		return nil, err
	}
	profiles := e.profiles(registry)
	for i := range profiles {
		profiles[i].Inventory = ageInventory(profiles[i].Inventory)
	}
	return profiles, nil
}
func ageInventory(inventory CloudInventory) CloudInventory {
	updated, err := time.Parse(time.RFC3339, inventory.UpdatedAt)
	inventory.Stale = inventory.Stale || err != nil || time.Since(updated) > 5*time.Minute
	return inventory
}
func (e *Engine) AddAccount(ctx context.Context, label, token string, sessionOnly bool) (AccountProfile, error) {
	e.accountMu.Lock()
	defer e.accountMu.Unlock()
	label = strings.TrimSpace(label)
	token = strings.TrimSpace(token)
	if label == "" || len(label) > 120 {
		return AccountProfile{}, errors.New("provide an account label of 1 to 120 characters")
	}
	if !strings.HasPrefix(token, "sbp_") || len(token) > 1024 || strings.ContainsAny(token, " \t\r\n") {
		return AccountProfile{}, errors.New("provide a Supabase personal access token from account token settings")
	}
	if strings.Contains(label, token) || Redact(label) != label {
		return AccountProfile{}, errors.New("account labels cannot contain credentials")
	}
	inventory, err := e.cloud.Inventory(ctx, token)
	if err != nil {
		state, message := accessFailure(err)
		return AccountProfile{}, &CloudAccessError{State: state, Message: message}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return AccountProfile{}, errors.New("could not generate account identity")
	}
	id := hex.EncodeToString(random)
	profile := AccountProfile{ID: id, Label: label, SessionOnly: sessionOnly, Inventory: inventory}
	profile.Inventory.AccountID = id
	err = e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		for _, existing := range e.profiles(registry) {
			if existing.Label == label {
				return errors.New("account label already exists; choose another label")
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if sessionOnly {
			e.sessions[id] = sessionAccount{Profile: profile, Token: token}
			return nil
		}
		profile.CredentialRef = Hash([]byte(e.Store.Root))[:16] + "/" + id
		if err := e.credentials.Set(profile.CredentialRef, token); err != nil {
			return ErrCredentialUnavailable
		}
		registry.Accounts = append(registry.Accounts, profile)
		if err := e.Store.Write(registry); err != nil {
			return errors.Join(err, e.credentials.Delete(profile.CredentialRef))
		}
		return nil
	})
	return profile, err
}
func (e *Engine) RemoveAccount(id string) error {
	e.accountMu.Lock()
	defer e.accountMu.Unlock()
	return e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		profile, err := findAccount(e.profiles(registry), id)
		if err != nil {
			return err
		}
		if profile.SessionOnly {
			delete(e.sessions, profile.ID)
			return nil
		}
		// Leave the profile intact if deleting the credential fails, so deletion can be retried.
		if err := e.credentials.Delete(profile.CredentialRef); err != nil {
			return ErrCredentialUnavailable
		}
		registry.Accounts = slices.DeleteFunc(registry.Accounts, func(a AccountProfile) bool { return a.ID == profile.ID })
		return e.Store.Write(registry)
	})
}
func (e *Engine) CloudList(ctx context.Context, id string, refresh bool) (CloudInventory, error) {
	e.accountMu.Lock()
	defer e.accountMu.Unlock()
	var result CloudInventory
	work := func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		profile, err := findAccount(e.profiles(registry), id)
		if err != nil {
			return err
		}
		result = ageInventory(profile.Inventory)
		if !refresh {
			return nil
		}
		var token string
		if profile.SessionOnly {
			token = e.sessions[profile.ID].Token
		} else {
			token, err = e.credentials.Get(profile.CredentialRef)
		}
		if err == nil {
			result, err = e.cloud.Inventory(ctx, token)
		}
		if err != nil {
			result = profile.Inventory
			result.Stale = true
			result.AccessState, result.Message = accessFailure(err)
			if errors.Is(err, ErrCredentialUnavailable) {
				result.AccessState = "credentials_unavailable"
				result.Message = ErrCredentialUnavailable.Error()
			}
		}
		result.AccountID = profile.ID
		profile.Inventory = result
		if profile.SessionOnly {
			session := e.sessions[profile.ID]
			session.Profile = profile
			e.sessions[profile.ID] = session
			return nil
		}
		for i := range registry.Accounts {
			if registry.Accounts[i].ID == profile.ID {
				registry.Accounts[i] = profile
			}
		}
		return e.Store.Write(registry)
	}
	if refresh {
		err := e.mutate(work)
		return result, err
	}
	err := work()
	return result, err
}
func (e *Engine) Associate(ctx context.Context, local, account, ref string) (LocalProjectAssociation, error) {
	// Revalidate access rather than trusting a cached project that may no longer be accessible.
	inventory, err := e.CloudList(ctx, account, true)
	if err != nil {
		return LocalProjectAssociation{}, err
	}
	if inventory.AccessState != "ready" || inventory.Stale {
		return LocalProjectAssociation{}, errors.New("refresh account access successfully before associating a local project")
	}
	if !slices.ContainsFunc(inventory.Projects, func(p CloudProject) bool { return p.Ref == ref }) {
		return LocalProjectAssociation{}, errors.New("cloud reference is not accessible through this account")
	}
	e.accountMu.Lock()
	defer e.accountMu.Unlock()
	var association LocalProjectAssociation
	err = e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		profile, err := findAccount(e.profiles(registry), inventory.AccountID)
		if err != nil {
			return err
		}
		if profile.Inventory.AccessState != "ready" {
			return errors.New("account access changed; refresh again")
		}
		project, err := find(registry, local)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		association = LocalProjectAssociation{LocalProjectID: project.ID, CloudRef: ref}
		registry.Associations = slices.DeleteFunc(registry.Associations, func(a LocalProjectAssociation) bool { return a.LocalProjectID == project.ID })
		registry.Associations = append(registry.Associations, association)
		return e.Store.Write(registry)
	})
	return association, err
}
func (e *Engine) Unassociate(local string) error {
	return e.mutateRegisteredProject(local, func(registry *Registry, project Project) {
		registry.Associations = slices.DeleteFunc(registry.Associations, func(a LocalProjectAssociation) bool { return a.LocalProjectID == project.ID })
	})
}
func (e *Engine) AssociationSuggestions(local string) ([]AssociationSuggestion, error) {
	registry, err := e.Registry()
	if err != nil {
		return nil, err
	}
	project, err := find(registry, local)
	if err != nil {
		return nil, err
	}
	suggestions := []AssociationSuggestion{}
	data, err := os.ReadFile(filepath.Join(project.Path, "supabase", ".temp", "project-ref"))
	if errors.Is(err, os.ErrNotExist) {
		return suggestions, nil
	}
	if err != nil {
		return nil, errors.New("could not read existing Supabase link")
	}
	ref := strings.TrimSpace(string(data))
	if !cloudRefPattern.MatchString(ref) {
		return suggestions, nil
	}
	accounts := []string{}
	for _, profile := range registry.Accounts {
		if slices.ContainsFunc(profile.Inventory.Projects, func(p CloudProject) bool { return p.Ref == ref }) {
			accounts = append(accounts, profile.ID)
		}
	}
	if len(accounts) > 0 {
		suggestions = append(suggestions, AssociationSuggestion{LocalProjectID: project.ID, CloudRef: ref, AccountIDs: accounts})
	}
	return suggestions, nil
}
func (e *Engine) DashboardURL(ctx context.Context, account, ref string) (string, error) {
	inventory, err := e.CloudList(ctx, account, false)
	if err != nil {
		return "", err
	}
	if !cloudRefPattern.MatchString(ref) || !slices.ContainsFunc(inventory.Projects, func(p CloudProject) bool { return p.Ref == ref }) {
		return "", errors.New("select a project from this account's inventory")
	}
	return "https://supabase.com/dashboard/project/" + ref, nil
}
