package scanners

import (
	"path/filepath"
	"strings"

	"github.com/codehound/codehound/core/pkg/scanners/iac"
	"github.com/codehound/codehound/core/pkg/scanners/sast"
	"github.com/codehound/codehound/core/pkg/scanners/sca"
	"github.com/codehound/codehound/core/pkg/scanners/secrets"
	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// ScanResult contains unified findings and evidences aggregated across all deterministic engines.
type ScanResult struct {
	Findings  []domain.Finding
	Evidences []domain.Evidence
	SBOM      *sca.CycloneDXBOM
}

// Coordinator orchestrates SAST, Secrets, SCA, and IaC scanning across repository files.
type Coordinator struct {
	sastEngine        *sast.Engine
	secretDetector    *secrets.Detector
	osvDB             *sca.OSVDatabase
	licenseChecker    *sca.LicenseChecker
	dockerfileScanner *iac.DockerfileScanner
	tfScanner         *iac.TerraformScanner
	k8sScanner        *iac.K8sScanner
}

// NewCoordinator initializes all deterministic security analyzers.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		sastEngine:        sast.NewEngine(),
		secretDetector:    secrets.NewDetector(),
		osvDB:             sca.NewOSVDatabase(),
		licenseChecker:    sca.NewLicenseChecker(),
		dockerfileScanner: iac.NewDockerfileScanner(),
		tfScanner:         iac.NewTerraformScanner(),
		k8sScanner:        iac.NewK8sScanner(),
	}
}

// ScanRepository runs all deterministic engines over repository files with automatic deduplication.
func (c *Coordinator) ScanRepository(tenantID, projectID, scanID uuid.UUID, files map[string][]byte) (*ScanResult, error) {
	findingMap := make(map[string]*domain.Finding)
	evidenceList := make([]domain.Evidence, 0)
	var allDependencies []sca.PackageDependency

	for rawPath, content := range files {
		cleanPath := filepath.Clean(rawPath)
		baseName := filepath.Base(cleanPath)
		ext := strings.ToLower(filepath.Ext(cleanPath))

		// 1. Secrets Scanning on all source files
		secFindings, secEvidences := c.secretDetector.ScanContent(tenantID, projectID, scanID, cleanPath, content)
		c.mergeFindings(findingMap, secFindings)
		evidenceList = append(evidenceList, secEvidences...)

		// 2. SAST Scanning on code files
		lang := detectLanguage(cleanPath, ext)
		if lang != "" {
			sastFindings, sastEvidences := c.sastEngine.ScanFile(tenantID, projectID, scanID, cleanPath, lang, content)
			c.mergeFindings(findingMap, sastFindings)
			evidenceList = append(evidenceList, sastEvidences...)
		}

		// 3. SCA & Dependency Parsing
		switch baseName {
		case "go.mod":
			deps, err := sca.ParseGoMod(content)
			if err == nil {
				allDependencies = append(allDependencies, deps...)
				vulns, vulnEvs := c.osvDB.MatchVulnerabilities(tenantID, projectID, scanID, cleanPath, deps)
				c.mergeFindings(findingMap, vulns)
				evidenceList = append(evidenceList, vulnEvs...)
			}
		case "package.json":
			deps, lic, err := sca.ParsePackageJSON(content)
			if err == nil {
				allDependencies = append(allDependencies, deps...)
				vulns, vulnEvs := c.osvDB.MatchVulnerabilities(tenantID, projectID, scanID, cleanPath, deps)
				c.mergeFindings(findingMap, vulns)
				evidenceList = append(evidenceList, vulnEvs...)

				if lic != "" {
					licFindings, licEvs := c.licenseChecker.CheckLicenseCompliance(tenantID, projectID, scanID, cleanPath, lic)
					c.mergeFindings(findingMap, licFindings)
					evidenceList = append(evidenceList, licEvs...)
				}
			}
		case "requirements.txt":
			deps, err := sca.ParseRequirementsTxt(content)
			if err == nil {
				allDependencies = append(allDependencies, deps...)
				vulns, vulnEvs := c.osvDB.MatchVulnerabilities(tenantID, projectID, scanID, cleanPath, deps)
				c.mergeFindings(findingMap, vulns)
				evidenceList = append(evidenceList, vulnEvs...)
			}
		}

		// 4. Infrastructure as Code (IaC) Scanning
		if baseName == "Dockerfile" || strings.HasPrefix(baseName, "Dockerfile.") {
			docFindings, docEvs := c.dockerfileScanner.ScanDockerfile(tenantID, projectID, scanID, cleanPath, content)
			c.mergeFindings(findingMap, docFindings)
			evidenceList = append(evidenceList, docEvs...)
		} else if ext == ".tf" {
			tfFindings, tfEvs := c.tfScanner.ScanTerraform(tenantID, projectID, scanID, cleanPath, content)
			c.mergeFindings(findingMap, tfFindings)
			evidenceList = append(evidenceList, tfEvs...)
		} else if (ext == ".yaml" || ext == ".yml") && isK8sManifest(content) {
			k8sFindings, k8sEvs := c.k8sScanner.ScanK8sManifest(tenantID, projectID, scanID, cleanPath, content)
			c.mergeFindings(findingMap, k8sFindings)
			evidenceList = append(evidenceList, k8sEvs...)
		}
	}

	// Generate SBOM
	sbom, _ := sca.GenerateCycloneDXBOM("Project-Artifact", allDependencies)

	// Flatten deduplicated findings
	resultFindings := make([]domain.Finding, 0, len(findingMap))
	for _, f := range findingMap {
		resultFindings = append(resultFindings, *f)
	}

	return &ScanResult{
		Findings:  resultFindings,
		Evidences: evidenceList,
		SBOM:      sbom,
	}, nil
}

func (c *Coordinator) mergeFindings(findingMap map[string]*domain.Finding, findings []domain.Finding) {
	for _, f := range findings {
		if existing, exists := findingMap[f.CanonicalKey]; exists {
			// Elevate severity if higher
			if severityRank(f.Severity) > severityRank(existing.Severity) {
				existing.Severity = f.Severity
			}
			if f.Confidence > existing.Confidence {
				existing.Confidence = f.Confidence
			}
		} else {
			fCopy := f
			findingMap[f.CanonicalKey] = &fCopy
		}
	}
}

func severityRank(s domain.FindingSeverity) int {
	switch s {
	case domain.FindingSeverityCritical:
		return 4
	case domain.FindingSeverityHigh:
		return 3
	case domain.FindingSeverityMedium:
		return 2
	case domain.FindingSeverityLow:
		return 1
	default:
		return 0
	}
}

func detectLanguage(path, ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	default:
		return ""
	}
}

func isK8sManifest(content []byte) bool {
	str := string(content)
	return strings.Contains(str, "apiVersion:") && strings.Contains(str, "kind:")
}
