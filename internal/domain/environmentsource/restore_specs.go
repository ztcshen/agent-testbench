package environmentsource

import (
	"path/filepath"
	"sort"
	"strings"
)

type SourcePolicy struct {
	RemoteOnly bool     `json:"remoteOnly"`
	OK         bool     `json:"ok"`
	Violations []string `json:"violations,omitempty"`
}

type PackageSpec struct {
	URL      string
	Branch   string
	Ref      string
	Checkout string
}

type RepoSpec struct {
	ServiceID string
	URL       string
	Branch    string
	Ref       string
	Checkout  string
}

func RepoSpecs(reposJSON string, servicesJSON string, workspace string) []RepoSpec {
	repoMap := jsonObjectString(reposJSON)
	services := jsonArrayString(servicesJSON)
	specByID := map[string]RepoSpec{}
	for id, raw := range repoMap {
		spec := RepoSpec{ServiceID: strings.TrimSpace(id)}
		if item, ok := raw.(map[string]any); ok {
			spec.URL = strings.TrimSpace(stringValue(item["url"]))
			spec.Branch = strings.TrimSpace(stringValue(item["branch"]))
			spec.Ref = strings.TrimSpace(stringValue(item["ref"]))
			spec.Checkout = strings.TrimSpace(stringValue(item["checkout"]))
		}
		if spec.ServiceID != "" {
			specByID[spec.ServiceID] = spec
		}
	}
	for _, raw := range services {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(stringValue(item["id"]))
		if id == "" {
			continue
		}
		spec := specByID[id]
		spec.ServiceID = id
		if value := strings.TrimSpace(stringValue(item["repo"])); value != "" {
			spec.URL = value
		}
		if value := strings.TrimSpace(stringValue(item["branch"])); value != "" {
			spec.Branch = value
		}
		if value := strings.TrimSpace(stringValue(item["ref"])); value != "" {
			spec.Ref = value
		}
		if value := strings.TrimSpace(stringValue(item["checkout"])); value != "" {
			spec.Checkout = value
		}
		specByID[id] = spec
	}
	ids := make([]string, 0, len(specByID))
	for id := range specByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]RepoSpec, 0, len(ids))
	for _, id := range ids {
		spec := specByID[id]
		if spec.Checkout == "" {
			spec.Checkout = filepath.Join(workspace, safeCheckoutDirName(id))
		} else if !filepath.IsAbs(spec.Checkout) {
			spec.Checkout = filepath.Join(workspace, spec.Checkout)
		}
		out = append(out, spec)
	}
	return out
}

func PackageSpecFromCompose(compose map[string]any, workspace string) PackageSpec {
	pkg := mapFromAny(compose["package"])
	spec := PackageSpec{
		URL:    strings.TrimSpace(stringValue(pkg["url"])),
		Branch: strings.TrimSpace(stringValue(pkg["branch"])),
		Ref:    strings.TrimSpace(stringValue(pkg["ref"])),
	}
	checkout := strings.TrimSpace(stringValue(pkg["checkout"]))
	if checkout == "" {
		checkout = "."
	}
	if filepath.IsAbs(checkout) {
		spec.Checkout = checkout
	} else {
		spec.Checkout = filepath.Join(workspace, checkout)
	}
	return spec
}

func SourcePolicyReport(specs []RepoSpec, remoteOnly bool) SourcePolicy {
	report := SourcePolicy{
		RemoteOnly: remoteOnly,
		OK:         true,
	}
	if !remoteOnly {
		return report
	}
	addViolation := func(label string, rawURL string) {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" || IsRemoteGitURL(rawURL) {
			return
		}
		report.OK = false
		report.Violations = append(report.Violations, label+" must use a remote Git URL, got local path/source: "+rawURL)
	}
	for _, spec := range specs {
		addViolation("component "+spec.ServiceID, spec.URL)
	}
	return report
}

func safeCheckoutDirName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "service"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
}
