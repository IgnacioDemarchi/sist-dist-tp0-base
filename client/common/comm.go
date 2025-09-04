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

// ---------- Batching ----------

type Batch struct {
	Type     string `json:"type"` // "BATCH_BET"
	AgencyID string `json:"agency_id"`
	Bets     []Bet  `json:"bets"`
}

type BatchAck struct {
	Type  string `json:"type"` // "ACK_BATCH"
	OK    bool   `json:"ok"`
	Count int    `json:"count"`
	Error string `json:"error,omitempty"`
}

const maxPayload = 8 * 1024 // must match server cap

func SendBatchBet(conn net.Conn, agencyID string, bets []Bet) (*BatchAck, [][]Bet, error) {
	// Try to send all; if too big, split until it fits.
	l, r := 0, len(bets)
	var lastGood int
	for l < r {
		payload := Batch{Type: "BATCH_BET", AgencyID: agencyID, Bets: bets[l:r]}
		js, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
		if len(js) <= maxPayload {
			// fits → send it
			if err := writeFrame(conn, js); err != nil {
				return nil, nil, err
			}
			resp, err := readFrame(conn)
			if err != nil {
				return nil, nil, err
			}
			var ack BatchAck
			if err := json.Unmarshal(resp, &ack); err != nil {
				return nil, nil, err
			}
			if ack.Type != "ACK_BATCH" {
				return nil, nil, fmt.Errorf("unexpected reply type: %s", ack.Type)
			}
			// remaining batches (if any) are none because we tried to send all at once
			return &ack, nil, nil
		}
		// Too big: reduce window by halving
		lastGood = (l + r) / 2
		if lastGood <= l {
			// Single item too large: fail fast (shouldn't happen with small bets)
			return nil, nil, fmt.Errorf("single bet too large to fit frame")
		}
		r = lastGood
	}
	return nil, nil, fmt.Errorf("unexpected batching logic failure")
}

// Helper: split bets into many <=8KB batches and send them one by one.
func SendBatches(conn net.Conn, agencyID string, bets []Bet) (*BatchAck, error) {
	i := 0
	for i < len(bets) {
		// Find the largest sub-slice starting at i that fits
		lo, hi := i+1, len(bets)+1
		best := -1
		for lo < hi {
			mid := (lo + hi) / 2
			payload := Batch{Type: "BATCH_BET", AgencyID: agencyID, Bets: bets[i:mid]}
			js, _ := json.Marshal(payload)
			if len(js) <= maxPayload {
				best = mid
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if best == -1 {
			return nil, fmt.Errorf("cannot fit any bet into frame at index %d", i)
		}
		// send this chunk
		chunk := bets[i:best]
		if _, _, err := SendBatchBet(conn, agencyID, chunk); err != nil {
			return nil, err
		}
		i = best
	}
	// Return a synthetic success ack with total count
	return &BatchAck{Type: "ACK_BATCH", OK: true, Count: len(bets)}, nil
}

type Done struct {
	Type     string `json:"type"` // "DONE"
	AgencyID string `json:"agency_id"`
}
type AckDone struct {
	Type string `json:"type"` // "ACK_DONE"
	OK   bool   `json:"ok"`
}
type GetWinners struct {
	Type     string `json:"type"` // "GET_WINNERS"
	AgencyID string `json:"agency_id"`
}
type WinnersResp struct {
	Type  string   `json:"type"` // "WINNERS"
	OK    bool     `json:"ok"`
	DNIs  []string `json:"dnis,omitempty"`
	Error string   `json:"error,omitempty"`
}

func SendDone(conn net.Conn, agencyID string) error {
	req := Done{Type: "DONE", AgencyID: agencyID}
	js, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := writeFrame(conn, js); err != nil {
		return err
	}
	resp, err := readFrame(conn)
	if err != nil {
		return err
	}
	var ack AckDone
	if err := json.Unmarshal(resp, &ack); err != nil {
		return err
	}
	if ack.Type != "ACK_DONE" || !ack.OK {
		return fmt.Errorf("ack_done not ok")
	}
	return nil
}

func RequestWinners(conn net.Conn, agencyID string) (*WinnersResp, error) {
	req := GetWinners{Type: "GET_WINNERS", AgencyID: agencyID}
	js, err := json.Marshal(req)
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
	var wr WinnersResp
	if err := json.Unmarshal(resp, &wr); err != nil {
		return nil, err
	}
	if wr.Type != "WINNERS" {
		return nil, fmt.Errorf("unexpected reply type: %s", wr.Type)
	}
	return &wr, nil
}
