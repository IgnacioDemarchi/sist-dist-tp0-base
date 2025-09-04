package common

import (
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	BatchMax      int
}

// Client Entity that encapsulates how
type Client struct {
	config ClientConfig
	conn   net.Conn
	stopCh chan struct{}
}

func loadAgencyBets(agencyID string) ([]Bet, error) {
	path := filepath.Join("/data", fmt.Sprintf("agency-%s.csv", agencyID))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	var out []Bet
	// Try to detect header row; if first cell isn't a digit DNI, assume header
	start := 0
	if len(rows) > 0 && len(rows[0]) > 0 {
		// very light heuristic: if header has "documento" or non-digit in col2, skip
		if rows[0][0] == "nombre" || rows[0][2] == "documento" {
			start = 1
		}
	}

	for i := start; i < len(rows); i++ {
		c := rows[i]
		// Defensive: support 4 or 5+ columns
		if len(c) < 5 {
			continue
		}
		num := 0
		fmt.Sscanf(c[4], "%d", &num)
		out = append(out, Bet{
			// Type set by sender
			AgencyID:   agencyID,
			Nombre:     c[0],
			Apellido:   c[1],
			Documento:  c[2],
			Nacimiento: c[3],
			Numero:     num,
		})
	}
	return out, nil
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

func (c *Client) StartClientLoop() {
	bets, err := loadAgencyBets(c.config.ID)
	if err != nil {
		log.Errorf("action: load_bets | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
	if c.config.BatchMax <= 0 {
		c.config.BatchMax = 100
	}

	// Walk the bets, sending batches (capped by BatchMax and <=8KB by SendBatches)
	sent := 0
	for sent < len(bets) {
		// stop quickly on SIGTERM
		select {
		case <-c.stopCh:
			log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
			return
		default:
		}

		end := sent + c.config.BatchMax
		if end > len(bets) {
			end = len(bets)
		}
		chunk := bets[sent:end]

		if err := c.createClientSocket(); err != nil {
			return
		}
		err := sendBatch(c.conn, c.config.ID, chunk)
		_ = c.conn.Close()
		if err != nil {
			log.Errorf("action: apuesta_enviada | result: fail | cantidad: %d | error: %v", len(chunk), err)
			return
		}

		log.Infof("action: apuesta_enviada | result: success | cantidad: %d", len(chunk))

		sent = end

		// Interruptible pacing between batches
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

	if err := c.createClientSocket(); err != nil {
		return
	}
	if err := SendDone(c.conn, c.config.ID); err != nil {
		_ = c.conn.Close()
		log.Errorf("action: done | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
	_ = c.conn.Close()

	// Immediately request winners; server will answer only after all 5 DONEs
	for {
		// allow graceful stop
		select {
		case <-c.stopCh:
			log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
			return
		default:
		}

		if err := c.createClientSocket(); err != nil {
			return
		}
		wr, err := GetWinners(c.conn, c.config.ID)
		_ = c.conn.Close()
		if err == nil {
			log.Infof("action: consulta_ganadores | result: success | cant_ganadores: %d", len(wr))
			break
		}
		// backoff a bit before retrying
		time.Sleep(200 * time.Millisecond)
	}

	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}
