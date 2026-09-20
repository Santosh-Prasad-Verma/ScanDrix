// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"bytes"
	"io"
	"os"
	"time"

	"golang.org/x/term"
)

// ReadStreamPayloadOptions controls timeout behavior when reading piped stdin data.
type ReadStreamPayloadOptions struct {
	NoDataTimeout       time.Duration
	BrokenStreamTimeout time.Duration
}

// ReadStreamPayload reads piped stdin content safely without blocking indefinitely if stdin is interactive.
func ReadStreamPayload(r io.Reader, opts ...ReadStreamPayloadOptions) (string, error) {
	// If reading from os.Stdin, check if it's connected to a terminal
	if f, ok := r.(*os.File); ok {
		if term.IsTerminal(int(f.Fd())) {
			return "", nil
		}
	}

	options := ReadStreamPayloadOptions{
		NoDataTimeout:       750 * time.Millisecond,
		BrokenStreamTimeout: 5 * time.Second,
	}
	if len(opts) > 0 {
		if opts[0].NoDataTimeout > 0 {
			options.NoDataTimeout = opts[0].NoDataTimeout
		}
		if opts[0].BrokenStreamTimeout > 0 {
			options.BrokenStreamTimeout = opts[0].BrokenStreamTimeout
		}
	}

	resultCh := make(chan struct {
		data string
		err  error
	}, 1)

	go func() {
		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		resultCh <- struct {
			data string
			err  error
		}{
			data: buf.String(),
			err:  err,
		}
	}()

	select {
	case res := <-resultCh:
		return res.data, res.err
	case <-time.After(options.BrokenStreamTimeout):
		return "", nil
	}
}
