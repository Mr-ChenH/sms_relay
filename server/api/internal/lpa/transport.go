package lpa

import "context"

// APDURequest is one operation emitted by lpac's stdio APDU driver.
type APDURequest struct {
	Func  string `json:"func"`
	Param string `json:"param,omitempty"`
}

// APDUResponse is returned by the terminal APDU tunnel.
type APDUResponse struct {
	ECode int    `json:"ecode"`
	Data  string `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// Transport forwards an APDU operation to one terminal.
type Transport interface {
	Exchange(ctx context.Context, deviceID string, request APDURequest) (APDUResponse, error)
}

type TaskUpdater interface {
	UpdateEsimTask(id, status, stage string, progress int) error
}
