//go:build windows

package graph

import gnet "github.com/shirou/gopsutil/v4/net"

// Listening lists the TCP sockets in the listen state with their owning
// pid, from Windows' own TCP table. It reads only; it opens no socket.
func Listening() ([]Listen, error) {
	conns, err := gnet.Connections("tcp")
	if err != nil {
		return nil, err
	}
	out := []Listen{}
	for _, c := range conns {
		if c.Status == "LISTEN" && c.Pid > 0 {
			out = append(out, Listen{PID: c.Pid, Proto: "tcp", Port: c.Laddr.Port})
		}
	}
	return out, nil
}
