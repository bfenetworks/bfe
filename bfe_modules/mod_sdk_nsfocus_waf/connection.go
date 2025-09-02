package mod_sdk_nsfocus_waf

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
)

type connection struct {
	conn   net.Conn
	client *http.Client
	server *Server
}

// newConn ceate new connection
func newConn(server *Server) (*connection, error) {
	// create connection
	conn, err := server.createFactory()
	if err != nil {
		return nil, err
	}
	// create http client
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return conn, nil
			},
		},
	}
	// create connection
	return &connection{
		conn:   conn,
		server: server,
		client: client,
	}, nil
}

// Close close connection
func (c *connection) Close() error {
	c.conn.Close()
	return nil
}

// detectRequst detect if request is valid
func (c *connection) detectRequst(req *http.Request, logId string) (*NsfocusWafResult, error) {
	// set remote addr
	newReq, err := createReqFromHTTPReq(req)
	if err != nil {
		return nil, err
	}
	// send request to
	resp, err := c.client.Do(newReq)
	if err != nil {
		return &NsfocusWafResult{
			LogID:  logId,
			Action: WAF_RESULT_PASS,
		}, nil
	}
	// close body
	defer resp.Body.Close()
	// check header
	if resp.StatusCode != http.StatusOK {
		return &NsfocusWafResult{
			LogID:  logId,
			Action: WAF_RESULT_BLOCK,
		}, nil
	}
	return &NsfocusWafResult{
		LogID:  logId,
		Action: WAF_RESULT_PASS,
	}, nil
}

// TODO: nsfocus waf dont support active-alive connection detection, should wait
// checkAlive check if connection alive
func (c *connection) keepAlive() error {
	// create request
	request := "HEAD / HTTP/1.0\r\n\r\n"
	// send request
	_, err := c.conn.Write([]byte(request))
	if err != nil {
		return err
	}
	// check response
	resp, err := http.ReadResponse(bufio.NewReader(c.conn), nil)
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
