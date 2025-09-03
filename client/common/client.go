package common

import (
	"fmt"
	"net"
	"os"
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

	nombre := os.Getenv("NOMBRE")
	apellido := os.Getenv("APELLIDO")
	dni := os.Getenv("DOCUMENTO")
	nacimiento := os.Getenv("NACIMIENTO")
	num := 0
	if v := os.Getenv("NUMERO"); v != "" {
		fmt.Sscanf(v, "%d", &num)
	}
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
		if err := c.createClientSocket(); err != nil {
			return
		}

		bet := &Bet{
			AgencyID:   c.config.ID,
			Nombre:     nombre,
			Apellido:   apellido,
			Documento:  dni,
			Nacimiento: nacimiento,
			Numero:     num,
		}

		ack, err := SendBet(c.conn, bet)
		_ = c.conn.Close()
		if err != nil || !ack.OK {
			if err == nil {
				err = fmt.Errorf(ack.Error)
			}
			log.Errorf("action: apuesta_enviada | result: fail | dni: %s | numero: %d | error: %v", dni, num, err)
			return
		}

		log.Infof("action: apuesta_enviada | result: success | dni: %s | numero: %d", dni, num)

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
