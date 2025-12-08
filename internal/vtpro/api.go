package vtpro

import (
	"time"

	"github.com/Norgate-AV/vtpc/internal/logger"
)

// VTProProcessAPI is a concrete implementation of the VTPro process management interface
// It wraps the Client for backward compatibility with the interface
type VTProProcessAPI struct {
	client *Client
}

func NewSimplProcessAPI(log logger.LoggerInterface) *VTProProcessAPI {
	return &VTProProcessAPI{
		client: NewClient(log),
	}
}

func (v VTProProcessAPI) FindWindow(targetPid uint32, debug bool) (uintptr, string) {
	return v.client.FindWindow(targetPid, debug)
}

func (v VTProProcessAPI) WaitForReady(hwnd uintptr, timeout time.Duration) bool {
	return v.client.WaitForReady(hwnd, timeout)
}
