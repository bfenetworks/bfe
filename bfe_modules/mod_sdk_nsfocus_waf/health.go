package mod_sdk_nsfocus_waf

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// TODO: in plugin mode, nsfocus waf current dont support to do health detection
// HealthCheck performs a health check on the given connection.
func HealthCheck(conn net.Conn) error {
	// create request
	request := "HEAD / HTTP/1.0\r\n\r\n"
	// send request
	_, err := conn.Write([]byte(request))
	if err != nil {
		return err
	}
	// check response
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return err
	}
	// close body
	defer resp.Body.Close()
	// check if status match
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("health check failed, status code: %d", resp.StatusCode)
	}
	return nil
}
