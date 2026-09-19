package dependency

import (
	"encoding/xml"
	"os"
	"strings"
)

type mavenDependencyResolver struct{}

var _ Resolver = mavenDependencyResolver{}

// NewMavenResolver returns the resolver for Maven project models.
func NewMavenResolver() Resolver { return mavenDependencyResolver{} }

func (mavenDependencyResolver) ID() string              { return "maven" }
func (mavenDependencyResolver) Languages() []string     { return []string{"java", "kotlin"} }
func (mavenDependencyResolver) ManifestNames() []string { return []string{"pom.xml"} }

func (mavenDependencyResolver) Resolve(manifestPath string) (Resolution, error) {
	project, err := parseMavenProject(manifestPath)
	if err != nil {
		return Resolution{}, err
	}
	properties := map[string]string{"project.version": project.Version, "pom.version": project.Version}
	for _, property := range project.Properties {
		properties[property.XMLName.Local] = strings.TrimSpace(property.Value)
	}
	var dependencies []Evidence
	for _, manifestDependency := range project.Dependencies {
		if manifestDependency.Scope == "test" || manifestDependency.Scope == "provided" {
			continue
		}
		version := resolveMavenValue(strings.TrimSpace(manifestDependency.Version), properties)
		if version == "" || strings.Contains(version, "${") {
			continue
		}
		module := strings.TrimSpace(manifestDependency.GroupID) + ":" + strings.TrimSpace(manifestDependency.ArtifactID)
		dependencies = append(dependencies, Evidence{Ecosystem: "maven", ImportName: module, Module: module, Version: version})
	}
	return Resolution{Applicable: true, Dependencies: dependencies}, nil
}

func (mavenDependencyResolver) Match(_ string, dependencies []Evidence) Match {
	matches := append([]Evidence(nil), dependencies...)
	sortEvidence(matches)
	return Match{Candidates: matches}
}

func (mavenDependencyResolver) DiscoverModule(manifestPath, relativeRoot string) (*ArtifactModule, error) {
	project, err := parseMavenProject(manifestPath)
	if err != nil {
		return nil, err
	}
	group, version := project.GroupID, project.Version
	if group == "" {
		group = project.Parent.GroupID
	}
	if version == "" {
		version = project.Parent.Version
	}
	if group == "" || project.ArtifactID == "" || version == "" {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "maven", Module: group + ":" + project.ArtifactID, Version: version, Root: relativeRoot}, nil
}

type mavenProject struct {
	XMLName    xml.Name `xml:"project"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Version    string   `xml:"version"`
	Parent     struct {
		GroupID string `xml:"groupId"`
		Version string `xml:"version"`
	} `xml:"parent"`
	Properties   []xmlProperty `xml:"properties>*"`
	Dependencies []struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
		Scope      string `xml:"scope"`
	} `xml:"dependencies>dependency"`
}

type xmlProperty struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

func parseMavenProject(path string) (mavenProject, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return mavenProject{}, err
	}
	var project mavenProject
	if err := xml.Unmarshal(content, &project); err != nil {
		return mavenProject{}, err
	}
	return project, nil
}

func resolveMavenValue(value string, properties map[string]string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return properties[strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")]
	}
	return value
}
