package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type CloudOrganization struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}
type CloudProject struct {
	Ref            string `json:"ref"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Region         string `json:"region"`
	Status         string `json:"status"`
}
type CloudInventory struct {
	AccountID     string              `json:"account_id"`
	Organizations []CloudOrganization `json:"organizations"`
	Projects      []CloudProject      `json:"projects"`
	UpdatedAt     string              `json:"updated_at"`
	Stale         bool                `json:"stale"`
	AccessState   string              `json:"access_state"`
	Message       string              `json:"message"`
}

// CloudReader returns only metadata; credentials and database content are excluded.
type CloudReader interface {
	Inventory(context.Context, string) (CloudInventory, error)
}
type CloudAccessError struct {
	State   string
	Message string
}

func (e *CloudAccessError) Error() string { return e.Message }
func cloudError(state, message string) error {
	return &CloudAccessError{State: state, Message: message}
}

// ManagementClient is deliberately limited to GET inventory endpoints.
type ManagementClient struct {
	client  *http.Client
	baseURL string
}

func NewManagementClient() *ManagementClient {
	return &ManagementClient{client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, baseURL: "https://api.supabase.com/v1"}
}
func (c *ManagementClient) get(ctx context.Context, token, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return cloudError("unavailable", "Could not prepare cloud inventory request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return cloudError("cancelled", "Cloud refresh cancelled; cached inventory retained")
		}
		return cloudError("unavailable", "Supabase could not be reached; check your connection and refresh")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
	case 401:
		return cloudError("credentials_expired", "Token expired or invalid; disconnect and reconnect with a new scoped token")
	case 403:
		return cloudError("permission_denied", "Token cannot read this inventory; grant Organizations Read and Organization Projects Read for the intended resources")
	case 429:
		return cloudError("rate_limited", "Supabase rate limit reached; wait before refreshing again")
	default:
		return cloudError("unavailable", "Supabase inventory is temporarily unavailable; refresh later")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return cloudError("unavailable", "Cloud inventory response exceeded its limit or could not be read")
	}
	if json.Unmarshal(data, target) != nil {
		return cloudError("unavailable", "Cloud inventory format was unexpected; cached inventory retained")
	}
	return nil
}

var cloudRefPattern = regexp.MustCompile(`^[a-z0-9]{20}$`)
var organizationSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (c *ManagementClient) Inventory(ctx context.Context, token string) (CloudInventory, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result := CloudInventory{Organizations: []CloudOrganization{}, Projects: []CloudProject{}, AccessState: "ready", Message: "Only resources accessible to this token are shown."}
	if err := c.get(ctx, token, "/organizations", &result.Organizations); err != nil {
		return result, err
	}
	if result.Organizations == nil {
		return result, cloudError("unavailable", "Cloud organization inventory format was unexpected")
	}
	seen := map[string]bool{}
	for i := range result.Organizations {
		org := &result.Organizations[i]
		if !organizationSlugPattern.MatchString(org.Slug) || org.ID == "" || strings.Contains(org.ID, token) || strings.Contains(org.Slug, token) {
			return result, cloudError("unavailable", "Cloud organization identity was unexpected")
		}
		org.Name = safeCloudText(org.Name, token)
		for offset, pages := 0, 0; ; pages++ {
			if pages >= 1000 {
				return result, cloudError("unavailable", "Cloud pagination limit reached; inventory was not saved")
			}
			var page struct {
				Projects   []CloudProject `json:"projects"`
				Pagination struct {
					Count  int `json:"count"`
					Limit  int `json:"limit"`
					Offset int `json:"offset"`
				} `json:"pagination"`
			}
			path := fmt.Sprintf("/organizations/%s/projects?limit=100&offset=%d", url.PathEscape(org.Slug), offset)
			if err := c.get(ctx, token, path, &page); err != nil {
				return result, err
			}
			if page.Projects == nil || page.Pagination.Count < 0 {
				return result, cloudError("unavailable", "Cloud project inventory format was unexpected")
			}
			for _, project := range page.Projects {
				if !cloudRefPattern.MatchString(project.Ref) {
					return result, cloudError("unavailable", "Cloud project reference was unexpected")
				}
				if seen[project.Ref] {
					return result, cloudError("unavailable", "Cloud pagination repeated a project; refresh again")
				}
				seen[project.Ref] = true
				project.OrganizationID = org.ID
				project.Name = safeCloudText(project.Name, token)
				project.Region = safeCloudText(project.Region, token)
				project.Status = safeCloudText(project.Status, token)
				result.Projects = append(result.Projects, project)
			}
			if len(page.Projects) == 0 && page.Pagination.Count > offset {
				return result, cloudError("unavailable", "Cloud pagination returned an incomplete inventory; refresh again")
			}
			offset += len(page.Projects)
			if len(page.Projects) == 0 || (page.Pagination.Count > 0 && offset >= page.Pagination.Count) || (page.Pagination.Count == 0 && len(page.Projects) < 100) {
				break
			}
		}
	}
	result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return result, nil
}
func safeCloudText(value, token string) string {
	return Redact(strings.ReplaceAll(value, token, "[REDACTED]"))
}
func accessFailure(err error) (string, string) {
	var failure *CloudAccessError
	if errors.As(err, &failure) {
		return failure.State, failure.Message
	}
	return "unavailable", "Cloud inventory is unavailable; refresh later"
}
