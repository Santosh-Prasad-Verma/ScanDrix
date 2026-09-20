// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package formatters

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// JUnitTestSuites represents the root <testsuites> XML element.
type JUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a <testsuite> XML element.
type JUnitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents an individual <testcase> XML element.
type JUnitTestCase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
}

// JUnitFailure represents a <failure> tag within a testcase.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// FormatJUnitXML serializes review issues into standard JUnit XML.
func FormatJUnitXML(w io.Writer, result *types.ReviewResult) error {
	if result == nil {
		result = &types.ReviewResult{}
	}

	failures := 0
	errorsCount := 0
	var testcases []JUnitTestCase

	for _, issue := range result.Issues {
		sev := strings.ToLower(string(issue.Severity))
		isErr := sev == "critical" || sev == "error"
		if isErr {
			errorsCount++
		} else {
			failures++
		}

		className := issue.File
		if className == "" {
			className = "ScanDrixReview"
		}

		testName := fmt.Sprintf("%s (line %d)", issue.Message, issue.Line)
		failureContent := fmt.Sprintf("File: %s:%d\nSeverity: %s\nCategory: %s\nRecommendation: %s\nSuggestion: %s",
			issue.File, issue.Line, issue.Severity, issue.Category, issue.Recommendation, issue.Suggestion)

		testcases = append(testcases, JUnitTestCase{
			Classname: className,
			Name:      testName,
			Time:      "0.01",
			Failure: &JUnitFailure{
				Message: issue.Message,
				Type:    string(issue.Severity),
				Content: failureContent,
			},
		})
	}

	suite := JUnitTestSuite{
		Name:      "ScanDrix AI Code Review",
		Tests:     len(testcases),
		Failures:  failures,
		Errors:    errorsCount,
		Time:      "0.50",
		TestCases: testcases,
	}

	suites := JUnitTestSuites{
		Name:     "ScanDrix Review Suite",
		Tests:    len(testcases),
		Failures: failures,
		Errors:   errorsCount,
		Time:     "0.50",
		Suites:   []JUnitTestSuite{suite},
	}

	w.Write([]byte(xml.Header))
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	return encoder.Encode(suites)
}
