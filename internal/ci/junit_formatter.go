// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package ci

import (
	"encoding/xml"
	"fmt"
	"time"
)

// JUnitTestSuites represents the root JUnit XML document.
type JUnitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	Name       string           `xml:"name,attr"`
	Tests      int              `xml:"tests,attr"`
	Failures   int              `xml:"failures,attr"`
	Errors     int              `xml:"errors,attr"`
	Time       string           `xml:"time,attr"`
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

type JUnitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

type JUnitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
}

type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// FormatJUnitXML generates standard JUnit XML format for CI test runners.
func FormatJUnitXML(findings []CIFinding, gatePassed bool, duration time.Duration) ([]byte, error) {
	timeSec := fmt.Sprintf("%.3f", duration.Seconds())

	suite := JUnitTestSuite{
		Name: "ScanDrix Automated Code Review",
		Time: timeSec,
	}

	failureCount := 0
	for _, f := range findings {
		isFailure := f.Severity == FailCritical || f.Severity == FailError

		tc := JUnitTestCase{
			Name:      fmt.Sprintf("[%s] %s (%s:%d)", f.Severity, f.RuleTitle, f.FilePath, f.StartLine),
			ClassName: fmt.Sprintf("scandrix.%s", f.Category),
			Time:      "0.001",
		}

		if isFailure {
			failureCount++
			tc.Failure = &JUnitFailure{
				Message: f.Message,
				Type:    string(f.Severity),
				Content: fmt.Sprintf("%s\nRule: %s\nFile: %s:%d-%d\nSuggestion: %s",
					f.Message, f.RuleID, f.FilePath, f.StartLine, f.EndLine, f.Suggestion),
			}
		}

		suite.TestCases = append(suite.TestCases, tc)
	}

	// If no findings, add single passing testcase
	if len(findings) == 0 {
		suite.TestCases = append(suite.TestCases, JUnitTestCase{
			Name:      "Quality Gate & Policy Enforcement",
			ClassName: "scandrix.QualityGate",
			Time:      timeSec,
		})
	}

	suite.Tests = len(suite.TestCases)
	suite.Failures = failureCount

	root := JUnitTestSuites{
		Name:       "ScanDrix Review Test Suites",
		Tests:      suite.Tests,
		Failures:   failureCount,
		Errors:     0,
		Time:       timeSec,
		TestSuites: []JUnitTestSuite{suite},
	}

	output, err := xml.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), output...), nil
}
