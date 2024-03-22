// SPDX-FileCopyrightText: 2021 M. Shulhan <ms@kilabit.info>
// SPDX-License-Identifier: GPL-3.0-or-later

package awwan

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"git.sr.ht/~shulhan/pakakeh.go/lib/http/sseclient"
)

// ExecResponse contains the request and output of command execution, from
// ExecRequest.
type ExecResponse struct {
	// Queue that push the each line in Output as event.
	eventq chan sseclient.Event

	// Copy of request.

	Mode      string `json:"mode"`
	Script    string `json:"script"`
	LineRange string `json:"line_range"`

	// ID of execution request that can be used to stream output or
	// got get full status.
	ID string `json:"id"`

	// BeginAt contains when the execution begin.
	BeginAt string `json:"begin_at"`

	// EndAt contains when the execution finished.
	EndAt string `json:"end_at"`

	Error string `json:"error"`

	Output []string `json:"output"`

	// mtxOutput protect read/write on Output.
	mtxOutput sync.Mutex
}

func newExecResponse(req *ExecRequest) (execRes *ExecResponse) {
	var now = timeNow().UTC()

	execRes = &ExecResponse{
		Mode:      req.Mode,
		Script:    req.Script,
		LineRange: req.LineRange,

		ID:      fmt.Sprintf(`%s:%s:%s:%d`, req.Mode, req.Script, req.LineRange, now.Unix()),
		BeginAt: now.Format(time.RFC3339),

		Output: make([]string, 0, 8),
		eventq: make(chan sseclient.Event, 512),
	}

	// Use the ExecResponse itself as handler for output.
	req.registerLogWriter(`response`, execRes)

	return execRes
}

// Write convert the raw output from execution into multiline string, and
// push it to field Output.
func (execRes *ExecResponse) Write(out []byte) (n int, err error) {
	if len(out) == 0 {
		return 0, nil
	}

	execRes.mtxOutput.Lock()
	var ev = sseclient.Event{
		Data: string(out),
		ID:   strconv.FormatInt(int64(len(execRes.Output)), 10),
	}

	execRes.Output = append(execRes.Output, ev.Data)

	select {
	case execRes.eventq <- ev:
	default:
	}
	execRes.mtxOutput.Unlock()

	return len(out), nil
}

// end mark the execution completed, possibly with error.
func (execRes *ExecResponse) end(execErr error) {
	var ev sseclient.Event

	if execErr != nil {
		execRes.Error = execErr.Error()
		ev.Data = execRes.Error

		select {
		case execRes.eventq <- ev:
		default:
		}
	}

	execRes.EndAt = timeNow().UTC().Format(time.RFC3339)

	ev.Type = `end`
	ev.Data = execRes.EndAt

	select {
	case execRes.eventq <- ev:
	default:
	}

	close(execRes.eventq)
}
