package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/aria/icons"
)

// Kind is what a project is, which decides its icon.
type Kind struct {
	Name string
	Icon tela.Icon
}

var (
	service  = Kind{"Service", icons.Server}
	database = Kind{"Database", icons.Database}
	website  = Kind{"Website", icons.Globe}
	library  = Kind{"Library", icons.Package}
	docs     = Kind{"Docs", icons.FileText}
)

// Project is an entry of the combobox.
type Project struct {
	Name string
	Kind Kind
}

// Projects are the thirty projects the combobox filters.
var Projects = []Project{
	{"acme-api", service}, {"auth-gateway", service}, {"billing-service", service},
	{"edge-router", service}, {"media-transcoder", service}, {"payments-worker", service},
	{"analytics-warehouse", database}, {"events-stream", database}, {"ledger-db", database},
	{"orders-db", database}, {"sessions-cache", database}, {"users-db", database},
	{"admin-console", website}, {"careers", website}, {"customer-portal", website},
	{"docs-portal", website}, {"marketing-site", website}, {"status-page", website},
	{"cli-tools", library}, {"design-system", library}, {"sdk-go", library},
	{"sdk-js", library}, {"telemetry-kit", library}, {"ui-icons", library},
	{"api-reference", docs}, {"changelog", docs}, {"onboarding-guide", docs},
	{"runbooks", docs}, {"security-policy", docs}, {"style-guide", docs},
}

// projectNames are the keys of the combobox: the names, which are unique.
var projectNames = func() []string {
	names := make([]string, len(Projects))
	for i, p := range Projects {
		names[i] = p.Name
	}
	return names
}()

// projectKind finds the kind of a project by its name.
func projectKind(name string) Kind {
	for _, p := range Projects {
		if p.Name == name {
			return p.Kind
		}
	}
	return Kind{}
}

// Status is where a deployment is.
type Status string

const (
	Succeeded Status = "Succeeded"
	Failed    Status = "Failed"
	Running   Status = "Running"
	Queued    Status = "Queued"
)

// Deployment is a row of the recent deployments.
type Deployment struct {
	Project  string
	Version  string
	Env      string
	Branch   string
	Author   string
	Status   Status
	Duration string
	Age      string
}

// Deployments are the recent deployments, newest first.
var Deployments = []Deployment{
	{"acme-api", "v2.14.0", "staging", "main", "Ana García", Running, "1m 48s", "just now"},
	{"customer-portal", "v5.2.3", "production", "release/5.2", "Mei Tanaka", Succeeded, "3m 12s", "24 min ago"},
	{"payments-worker", "v1.9.1", "production", "fix/retry-backoff", "Omar Haddad", Failed, "2m 05s", "1 h ago"},
	{"design-system", "v8.0.0", "staging", "main", "Sofia Rossi", Succeeded, "4m 31s", "3 h ago"},
	{"auth-gateway", "v3.7.4", "production", "main", "Kwame Mensah", Succeeded, "2m 49s", "5 h ago"},
}

// Member is a person who can be notified.
type Member struct {
	Name, Role string
	// Chart is the chart token (1 to 5) that colors the avatar.
	Chart int
}

// Members are the team the listbox selects from.
var Members = []Member{
	{"Ana García", "Platform lead", 1},
	{"Mei Tanaka", "Site reliability", 2},
	{"Omar Haddad", "Backend engineer", 3},
	{"Lena Fischer", "Frontend engineer", 4},
	{"Sofia Rossi", "Product designer", 5},
	{"Kwame Mensah", "Security engineer", 1},
}

// memberNames are the keys of the listbox.
var memberNames = func() []string {
	names := make([]string, len(Members))
	for i, m := range Members {
		names[i] = m.Name
	}
	return names
}()

// Activity is a line of the activity tab.
type Activity struct {
	Who, What, Where, When string
}

// ActivityLog is the activity tab.
var ActivityLog = []Activity{
	{"Mei Tanaka", "deployed customer-portal v5.2.3 to", "production", "24 min ago"},
	{"Omar Haddad", "rolled back payments-worker to v1.9.0 on", "production", "1 h ago"},
	{"Sofia Rossi", "published design-system v8.0.0 to", "staging", "3 h ago"},
	{"Kwame Mensah", "rotated the signing keys of auth-gateway on", "production", "5 h ago"},
	{"Ana García", "opened the release train for acme-api on", "staging", "yesterday"},
}

// Alert is a card of the alerts tab.
type Alert struct {
	Title, Detail string
	Severe        bool
}

// Alerts is the alerts tab.
var Alerts = []Alert{
	{"payments-worker failed its health check", "The 1.9.1 rollout stopped at 2 of 6 replicas after the retry queue grew.", true},
	{"Certificate expires in 12 days", "The certificate of status-page renews automatically on Nov 2.", false},
	{"Build cache is 91% full", "The oldest layers are evicted first. Nothing to do.", false},
}

// Environments are the targets of a deployment.
var Environments = []string{"production", "staging", "preview", "development"}

// EnvLabel is the text of an environment.
func EnvLabel(key string) string {
	if key == "" {
		return ""
	}
	return strings.ToUpper(key[:1]) + key[1:]
}
