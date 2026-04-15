// Package snyk provides a client for the Snyk REST API.
package snyk

import (
	"time"
)

// Credentials holds Snyk authentication details.
type Credentials struct {
	Token string // Snyk API token
	OrgID string // Snyk organization ID
	Site  string // API host (default "api.snyk.io")
}

// BaseURL returns the Snyk API base URL.
func (c *Credentials) BaseURL() string {
	site := c.Site
	if site == "" {
		site = "api.snyk.io"
	}
	return "https://" + site
}

// Org represents a Snyk organization.
type Org struct {
	ID   string
	Name string
}

// IssueFilter holds optional filters applied when querying issues from the Snyk API.
type IssueFilter struct {
	// ScanItemType restricts results to a specific scan origin, e.g. "cli" for CI/CLI scans.
	// Leave empty to return issues from all scan types.
	ScanItemType string
}

// OpenCounts holds the current total of open issues broken down by severity and fixability.
type OpenCounts struct {
	Critical, High, Medium, Low, Total int
	Fixable, Unfixable                 int
	Ignored, IgnoredFixable, IgnoredUnfixable int
	// Exploitable counts ("Proof of Concept" or "Attacked" maturity)
	ExploitableCritical, ExploitableHigh, ExploitableMedium, ExploitableLow int
	ExploitableFixable, ExploitableUnfixable                                 int
	ExploitableIgnoredFixable, ExploitableIgnoredUnfixable                   int
}

// Issue represents a Snyk vulnerability issue.
type Issue struct {
	ID             string
	Title          string
	Severity       string // critical, high, medium, low
	IssueType      string
	Status         string
	IsFixable      bool
	IsIgnored      bool
	Exploitability string // "Not Defined", "Proof of Concept", "Attacked"
	CreatedAt      time.Time
	ResolvedAt     time.Time
}

// coordinate holds fix information for one affected package coordinate.
type coordinate struct {
	IsFixableManually bool `json:"is_fixable_manually"`
	IsFixableSnyk     bool `json:"is_fixable_snyk"`
	IsFixableUpstream bool `json:"is_fixable_upstream"`
	IsPatchable       bool `json:"is_patchable"`
	IsPinnable        bool `json:"is_pinnable"`
	IsUpgradeable     bool `json:"is_upgradeable"`
}

// maturityLevel represents one entry in exploit_details.maturity_levels.
type maturityLevel struct {
	Level  string `json:"level"`
	Format string `json:"format"` // e.g. "CVSSv3", "CVSSv4"
	Type   string `json:"type"`   // e.g. "primary", "secondary"
}

// exploitDetails holds exploit maturity from the issues API response.
// MaturityLevels is the current field (array, one entry per CVSS format).
// Maturity is the legacy scalar field kept for backward compatibility.
type exploitDetails struct {
	Maturity       string          `json:"maturity"`
	MaturityLevels []maturityLevel `json:"maturity_levels"`
}

// issueAttributes holds attributes from the issues API response.
type issueAttributes struct {
	Title                  string         `json:"title"`
	EffectiveSeverityLevel string         `json:"effective_severity_level"`
	Type                   string         `json:"type"`
	Status                 string         `json:"status"`
	Ignored                bool           `json:"ignored"`
	CreatedAt              string         `json:"created_at"`
	ResolvedAt             string         `json:"resolved_at"`
	Coordinates            []coordinate   `json:"coordinates"`
	ExploitDetails         exploitDetails `json:"exploit_details"`
}

// issueRelationships holds relationship references from the issues API response.
type issueRelationships struct {
	ScanItem struct {
		Data struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	} `json:"scan_item"`
}

// issueData represents a single item in the issues API response.
type issueData struct {
	ID            string             `json:"id"`
	Attributes    issueAttributes    `json:"attributes"`
	Relationships issueRelationships `json:"relationships"`
}

// projectData represents a single item in the projects API response.
type projectData struct {
	ID            string `json:"id"`
	Relationships struct {
		Target struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"target"`
	} `json:"relationships"`
}

// projectListResponse is the raw API response for listing projects.
type projectListResponse struct {
	Data  []projectData `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// issueListResponse is the raw API response for listing issues.
type issueListResponse struct {
	Data  []issueData `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// orgAttributes holds attributes from the orgs API response.
type orgAttributes struct {
	Name string `json:"name"`
}

// orgData represents a single item in the orgs API response.
type orgData struct {
	ID         string        `json:"id"`
	Attributes orgAttributes `json:"attributes"`
}

// orgListResponse is the raw API response for listing orgs.
type orgListResponse struct {
	Data  []orgData `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// selfResponse is the raw API response for the self endpoint.
type selfResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}
