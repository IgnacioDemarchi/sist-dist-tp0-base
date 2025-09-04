package common

import (
	"encoding/csv"
	"fmt"
	"io"
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

// NewClient Initializes a new client
func NewClient(cfg ClientConfig) *Client {
	return &Client{config: cfg, stopCh: make(chan struct{})}
}

// CreateClientSocket Initializes client socket
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

// streamAgencyBets reads /data/agency-{ID}.csv incrementally and invokes emit(chunk)
// with up to maxChunk Bet items (last chunk may be smaller). It never loads the full file.
func streamAgencyBets(agencyID string, maxChunk int, emit func([]Bet) error) error {
	if maxChunk <= 0 {
		maxChunk = 100
	}

	path := filepath.Join("/data", fmt.Sprintf("agency-%s.csv", agencyID))
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerant

	// Read first row to decide header vs data — do NOT pre-fill or consume more.
	first, err := r.Read()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	isHeader := false
	if len(first) >= 3 {
		if first[0] == "nombre" || first[2] == "documento" {
			isHeader = true
		}
	}

	buf := make([]Bet, 0, maxChunk)

	enqueue := func(c []string) {
		if len(c) < 5 {
			return
		}
		var num int
		fmt.Sscanf(c[4], "%d", &num)
		buf = append(buf, Bet{
			AgencyID:   agencyID,
			Nombre:     c[0],
			Apellido:   c[1],
			Documento:  c[2],
			Nacimiento: c[3],
			Numero:     num,
		})
	}

	// Seed with first row if it's data.
	if !isHeader {
		enqueue(first)
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			// flush remaining
			if len(buf) > 0 {
				if err := emit(buf); err != nil {
					return err
				}
			}
			return nil
		}
		if err != nil {
			return err
		}

		enqueue(row)

		if len(buf) >= maxChunk {
			if err := emit(buf); err != nil {
				return err
			}
			buf = buf[:0] // reuse capacity
		}
	}
}

// StartClientLoop streams the CSV and sends batches as they’re produced.
// Memory stays bounded to ~BatchMax bets + protocol buffers.
func (c *Client) StartClientLoop() {
	if c.config.BatchMax <= 0 {
		c.config.BatchMax = 100
	}

	sentTotal := 0
	stopped := false

	emit := func(chunk []Bet) error {
		// Allow graceful stop between chunks
		select {
		case <-c.stopCh:
			stopped = true
			return io.EOF
		default:
		}

		// Create connection per chunk, send (auto-split to ≤8KB if needed), close
		if err := c.createClientSocket(); err != nil {
			return err
		}
		err := sendBatch(c.conn, c.config.ID, chunk)
		_ = c.conn.Close()
		if err != nil {
			log.Errorf("action: apuesta_enviada | result: fail | cantidad: %d | error: %v", len(chunk), err)
			return err
		}

		log.Infof("action: apuesta_enviada | result: success | cantidad: %d", len(chunk))
		sentTotal += len(chunk)

		// Pacing, but interruptible
		timer := time.NewTimer(c.config.LoopPeriod)
		select {
		case <-c.stopCh:
			if !timer.Stop() {
				<-timer.C
			}
			stopped = true
			return io.EOF
		case <-timer.C:
		}
		return nil
	}

	// Stream the CSV and send as we go
	if err := streamAgencyBets(c.config.ID, c.config.BatchMax, emit); err != nil && err != io.EOF {
		log.Errorf("action: load_bets | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
	if stopped {
		log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
		return
	}

	// DONE + winners polling
	if err := c.createClientSocket(); err != nil {
		return
	}
	if err := SendDone(c.conn, c.config.ID); err != nil {
		_ = c.conn.Close()
		log.Errorf("action: done | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}
	_ = c.conn.Close()

	for {
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
		time.Sleep(200 * time.Millisecond)
	}

	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}
