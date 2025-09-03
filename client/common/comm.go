package common

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

func writeFrame(conn net.Conn, payload []byte) error {
	if len(payload) > 8*1024 { // cop-out limit; increase if you like
		return fmt.Errorf("payload too big: %d", len(payload))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	// sendall: header first, then body
	if _, err := conn.Write(hdr[:]); err != nil {
		return err
	}
	off := 0
	for off < len(payload) {
		n, err := conn.Write(payload[off:])
		if err != nil {
			return err
		}
		off += n
	}
	return nil
}

func readFrame(conn net.Conn) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	sz := binary.BigEndian.Uint32(hdr[:])
	if sz == 0 || sz > 8*1024 {
		return nil, fmt.Errorf("invalid frame size: %d", sz)
	}
	buf := make([]byte, sz)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// --- JSON message types ---
type Bet struct {
	Type       string `json:"type"` // "BET"
	AgencyID   string `json:"agency_id"`
	Nombre     string `json:"nombre"`
	Apellido   string `json:"apellido"`
	Documento  string `json:"documento"`
	Nacimiento string `json:"nacimiento"` // "YYYY-MM-DD"
	Numero     int    `json:"numero"`
}

type Ack struct {
	Type  string `json:"type"` // "ACK"
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func SendBet(conn net.Conn, b *Bet) (*Ack, error) {
	b.Type = "BET"
	js, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	if err := writeFrame(conn, js); err != nil {
		return nil, err
	}
	resp, err := readFrame(conn)
	if err != nil {
		return nil, err
	}
	var ack Ack
	if err := json.Unmarshal(resp, &ack); err != nil {
		return nil, err
	}
	if ack.Type != "ACK" {
		return nil, fmt.Errorf("unexpected reply type: %s", ack.Type)
	}
	return &ack, nil
}
