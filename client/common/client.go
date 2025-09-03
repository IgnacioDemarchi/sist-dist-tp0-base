package common

import (
	"bufio"
	"fmt"
	"net"
	"time"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("log")

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
}

// Client Entity that encapsulates how
type Client struct {
	config ClientConfig
	conn   net.Conn
	stopCh chan struct{}
}

// Called on SIGTERM to stop gracefully
func (c *Client) Close() {
	select {
	case <-c.stopCh:
		// already closed
	default:
		close(c.stopCh)
	}
	if c.conn != nil {
		_ = c.conn.Close() // unblock any pending read/write
	}
}

// NewClient Initializes a new client receiving the configuration
// as a parameter
func NewClient(cfg ClientConfig) *Client {
	return &Client{config: cfg, stopCh: make(chan struct{})}
}

// CreateClientSocket Initializes client socket. In case of
// failure, error is printed in stdout/stderr and exit 1
// is returned
func (c *Client) createClientSocket() error {
	conn, err := net.Dial("tcp", c.config.ServerAddress)
	if err != nil {
		log.Criticalf(
			"action: connect | result: fail | client_id: %v | error: %v",
			c.config.ID,
			err,
		)
		return err
	}
	c.conn = conn
	return nil
}

// StartClientLoop Send messages to the client until some time threshold is met
func (c *Client) StartClientLoop() {
	// There is an autoincremental msgID to identify every message sent
	// Messages if the message amount threshold has not been surpassed
	for msgID := 1; msgID <= c.config.LoopAmount; msgID++ {

		select {
		case <-c.stopCh:
			log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
			return
		default:
		}

		// Create the connection the server in every loop iteration. Send an
		c.createClientSocket()

		// Write full line (avoid short-writes)
		bw := bufio.NewWriter(c.conn)
		line := fmt.Sprintf("[CLIENT %v] Message N°%v\n", c.config.ID, msgID)
		if _, err := bw.WriteString(line); err != nil {
			log.Errorf("action: send_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
			_ = c.conn.Close()
			return
		}
		if err := bw.Flush(); err != nil {
			log.Errorf("action: send_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
			_ = c.conn.Close()
			return
		}

		msg, err := bufio.NewReader(c.conn).ReadString('\n')
		c.conn.Close()

		if err != nil {
			log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v",
				c.config.ID,
				err,
			)
			return
		}

		log.Infof("action: receive_message | result: success | client_id: %v | msg: %v",
			c.config.ID,
			msg,
		)

		// Interruptible sleep so SIGTERM doesn’t wait a full period
		timer := time.NewTimer(c.config.LoopPeriod)
		select {
		case <-c.stopCh:
			if !timer.Stop() {
				<-timer.C
			}
			log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
			return
		case <-timer.C:
		}
	}
	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}
