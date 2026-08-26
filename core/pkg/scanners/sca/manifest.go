package sca

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PackageDependency represents a single direct or transitive dependency.
type PackageDependency struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"` // Go, npm, PyPI, Crates
	License   string `json:"license,omitempty"`
	Direct    bool   `json:"direct"`
}

// CycloneDXBOM represents a CycloneDX v1.5 SBOM JSON schema.
type CycloneDXBOM struct {
	BOMFormat    string              `json:"bomFormat"`
	SpecVersion  string              `json:"specVersion"`
	SerialNumber string              `json:"serialNumber"`
	Version      int                 `json:"version"`
	Metadata     BOMMetadata         `json:"metadata"`
	Components   []CycloneDXComponent `json:"components"`
}

type BOMMetadata struct {
	Timestamp string   `json:"timestamp"`
	Tools     []BOMTool `json:"tools"`
}

type BOMTool struct {
	Vendor  string `json:"vendor"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CycloneDXComponent struct {
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	PURL     string            `json:"purl"`
	Licenses []ComponentLicense `json:"licenses,omitempty"`
}

type ComponentLicense struct {
	License LicenseDetail `json:"license"`
}

type LicenseDetail struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ParseGoMod extracts dependencies from a go.mod file.
func ParseGoMod(content []byte) ([]PackageDependency, error) {
	var deps []PackageDependency
	scanner := bufio.NewScanner(bytes.NewReader(content))
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if line == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		if inRequireBlock {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				isIndirect := strings.Contains(line, "// indirect")
				deps = append(deps, PackageDependency{
					Name:      parts[0],
					Version:   parts[1],
					Ecosystem: "Go",
					Direct:    !isIndirect,
				})
			}
		} else if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(line[8:])
			if len(parts) >= 2 {
				isIndirect := strings.Contains(line, "// indirect")
				deps = append(deps, PackageDependency{
					Name:      parts[0],
					Version:   parts[1],
					Ecosystem: "Go",
					Direct:    !isIndirect,
				})
			}
		}
	}
	return deps, scanner.Err()
}

// PackageJSONSchema represents Node.js package.json.
type PackageJSONSchema struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	License         string            `json:"license"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// ParsePackageJSON extracts dependencies from a package.json file.
func ParsePackageJSON(content []byte) ([]PackageDependency, string, error) {
	var pkg PackageJSONSchema
	if err := json.Unmarshal(content, &pkg); err != nil {
		return nil, "", fmt.Errorf("failed to parse package.json: %w", err)
	}

	var deps []PackageDependency
	for name, ver := range pkg.Dependencies {
		cleanVer := strings.TrimPrefix(strings.TrimPrefix(ver, "^"), "~")
		deps = append(deps, PackageDependency{
			Name:      name,
			Version:   cleanVer,
			Ecosystem: "npm",
			Direct:    true,
		})
	}
	for name, ver := range pkg.DevDependencies {
		cleanVer := strings.TrimPrefix(strings.TrimPrefix(ver, "^"), "~")
		deps = append(deps, PackageDependency{
			Name:      name,
			Version:   cleanVer,
			Ecosystem: "npm",
			Direct:    false,
		})
	}

	return deps, pkg.License, nil
}

// ParseRequirementsTxt extracts dependencies from Python requirements.txt.
func ParseRequirementsTxt(content []byte) ([]PackageDependency, error) {
	var deps []PackageDependency
	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "==")
		if len(parts) == 2 {
			deps = append(deps, PackageDependency{
				Name:      strings.TrimSpace(parts[0]),
				Version:   strings.TrimSpace(parts[1]),
				Ecosystem: "PyPI",
				Direct:    true,
			})
		} else if strings.Contains(line, ">=") {
			gteParts := strings.Split(line, ">=")
			if len(gteParts) == 2 {
				deps = append(deps, PackageDependency{
					Name:      strings.TrimSpace(gteParts[0]),
					Version:   strings.TrimSpace(gteParts[1]),
					Ecosystem: "PyPI",
					Direct:    true,
				})
			}
		} else {
			deps = append(deps, PackageDependency{
				Name:      line,
				Version:   "*",
				Ecosystem: "PyPI",
				Direct:    true,
			})
		}
	}
	return deps, scanner.Err()
}

// GenerateCycloneDXBOM converts discovered dependencies into a CycloneDX SBOM.
func GenerateCycloneDXBOM(projectName string, deps []PackageDependency) (*CycloneDXBOM, error) {
	bom := &CycloneDXBOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  "1.5",
		SerialNumber: "urn:uuid:" + uuid.New().String(),
		Version:      1,
		Metadata: BOMMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Tools: []BOMTool{
				{
					Vendor:  "CodeHound",
					Name:    "CodeHound-SBOM-Syft-Core",
					Version: "1.0.0",
				},
			},
		},
		Components: make([]CycloneDXComponent, 0, len(deps)),
	}

	for _, dep := range deps {
		purl := fmt.Sprintf("pkg:%s/%s@%s", strings.ToLower(dep.Ecosystem), dep.Name, dep.Version)
		comp := CycloneDXComponent{
			Type:    "library",
			Name:    dep.Name,
			Version: dep.Version,
			PURL:    purl,
		}
		if dep.License != "" {
			comp.Licenses = []ComponentLicense{
				{
					License: LicenseDetail{
						ID: dep.License,
					},
				},
			}
		}
		bom.Components = append(bom.Components, comp)
	}

	return bom, nil
}
