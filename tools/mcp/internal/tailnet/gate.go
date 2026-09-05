package tailnet

import (
	"context"
	"fmt"
	"net"
	"time"
)

const ErrNotOnTailnet = "não estou na tailnet — rode tailscale up"

func Require(ctx context.Context, addr string, timeout time.Duration) error {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("%s (%v)", ErrNotOnTailnet, err)
	}
	conn.Close()
	return nil
}
