// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RFC2119Level categorizes the normative strength of an atomic requirement.
type RFC2119Level string

const (
	NormativeMust      RFC2119Level = "MUST"
	NormativeMustNot   RFC2119Level = "MUST NOT"
	NormativeRequired  RFC2119Level = "REQUIRED"
	NormativeShall     RFC2119Level = "SHALL"
	NormativeShallNot  RFC2119Level = "SHALL NOT"
	NormativeShould    RFC2119Level = "SHOULD"
	NormativeShouldNot RFC2119Level = "SHOULD NOT"
	NormativeNever     RFC2119Level = "NEVER"
	NormativeAlways    RFC2119Level = "ALWAYS"
	NormativeMay       RFC2119Level = "MAY"
)

// DrixyRuleExample represents positive (correct) or negative (incorrect) code snippets.
type DrixyRuleExample struct {
	Snippet     string `json:"snippet"`
	IsCorrect   bool   `json:"is_correct"`
	Description string `json:"description,omitempty"`
}

// DrixyRuleAtom is one atomic requirement decomposed from a compound rule.
// Atoms are the unit of review deliberation and T0 mechanical compilation.
type DrixyRuleAtom struct {
	ID             string                 `json:"id"`
	ParentRuleID   uuid.UUID              `json:"parent_rule_id"`
	ParentSlug     string                 `json:"parent_slug,omitempty"`
	Index          int                    `json:"index"`
	Title          string                 `json:"title"`
	Spec           string                 `json:"spec"`
	Severity       models.FindingSeverity `json:"severity"`
	Category       string                 `json:"category"`
	NormativeLevel RFC2119Level           `json:"normative_level,omitempty"`
	Examples       []DrixyRuleExample     `json:"examples,omitempty"`
	Detector       *CompiledRuleDetector  `json:"detector,omitempty"`
	DeclineReason  string                 `json:"decline_reason,omitempty"`
	PathGlobs      []string               `json:"path_globs,omitempty"`
	Remediation    string                 `json:"remediation,omitempty"`
}

// ToRuleAtom converts the deep atom into the lightweight legacy RuleAtom struct.
func (a *DrixyRuleAtom) ToRuleAtom() RuleAtom {
	return RuleAtom{
		ID:        a.ID,
		Title:     a.Title,
		Invariant: a.Spec,
		Severity:  a.Severity,
	}
}

// DrixyRuleAtoms is the container of decomposed atomic requirements for a rule.
type DrixyRuleAtoms struct {
	Items       []*DrixyRuleAtom `json:"items"`
	SourceHash  string           `json:"source_hash"`
	GeneratedAt time.Time        `json:"generated_at"`
	Model       string           `json:"model,omitempty"`
}

// ComputeAtomsSourceHash computes a deterministic sha256 across rule description and examples.
func ComputeAtomsSourceHash(ruleText string, examples []DrixyRuleExample) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(ruleText)))
	for _, ex := range examples {
		h.Write([]byte(fmt.Sprintf("\n%t:%s", ex.IsCorrect, strings.TrimSpace(ex.Snippet))))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IsFresh checks whether the atoms decomposition matches the current rule text and examples.
func (a *DrixyRuleAtoms) IsFresh(ruleText string, examples []DrixyRuleExample) bool {
	if a == nil || len(a.Items) == 0 || a.SourceHash == "" {
		return false
	}
	expected := ComputeAtomsSourceHash(ruleText, examples)
	return a.SourceHash == expected
}

// MechanicalAtoms returns all decomposed atoms that have a valid compiled T0 detector.
func (a *DrixyRuleAtoms) MechanicalAtoms() []*DrixyRuleAtom {
	if a == nil {
		return nil
	}
	var res []*DrixyRuleAtom
	for _, it := range a.Items {
		if it.Detector != nil && it.Detector.Pattern != "" {
			res = append(res, it)
		}
	}
	return res
}

// SemanticAtoms returns all decomposed atoms that require deep agent deliberation.
func (a *DrixyRuleAtoms) SemanticAtoms() []*DrixyRuleAtom {
	if a == nil {
		return nil
	}
	var res []*DrixyRuleAtom
	for _, it := range a.Items {
		if it.Detector == nil || it.Detector.Pattern == "" {
			res = append(res, it)
		}
	}
	return res
}

// ToRuleAtoms converts all atoms in the collection to RuleAtom slices.
func (a *DrixyRuleAtoms) ToRuleAtoms() []RuleAtom {
	if a == nil {
		return nil
	}
	res := make([]RuleAtom, 0, len(a.Items))
	for _, it := range a.Items {
		res = append(res, it.ToRuleAtom())
	}
	return res
}

// SerializeAtoms converts atoms to JSON bytes.
func (a *DrixyRuleAtoms) SerializeAtoms() ([]byte, error) {
	return json.Marshal(a)
}

// DeserializeAtoms parses JSON bytes into a DrixyRuleAtoms collection.
func DeserializeAtoms(data []byte) (*DrixyRuleAtoms, error) {
	var res DrixyRuleAtoms
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
