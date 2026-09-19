// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"

	"github.com/scandrix/backend/internal/cli/utils"
)

// TrialStatus represents anonymous/trial review quota information.
type TrialStatus = utils.TrialStatus

// CheckTrialStatus retrieves the trial usage limits for the current anonymous device ID.
func (c *Client) CheckTrialStatus(ctx context.Context, fingerprint ...string) (*TrialStatus, error) {
	fp := ""
	if len(fingerprint) > 0 && fingerprint[0] != "" {
		fp = fingerprint[0]
	} else {
		fp = utils.GetDeviceFingerprint()
	}
	status, err := c.GetTrialStatus(ctx, fp)
	if err != nil {
		return &TrialStatus{
			ReviewsLimit:  5,
			Remaining:     5,
			RemainingUses: 5,
			Allowed:       true,
		}, nil
	}
	return status, nil
}
