package common

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

// ------- framing --------
func writeFrame(conn net.Conn, payload []byte) error {
	if len(payload) > 8*1024 {
		return fmt.Errorf("payload too big: %d", len(payload))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
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

// ------- domain types -------
type Bet struct {
	AgencyID   string
	Nombre     string
	Apellido   string
	Documento  string
	Nacimiento string // YYYY-MM-DD
	Numero     int
}

// ------- line encoding / parsing -------
func encodeBetLine(b Bet) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		b.AgencyID, b.Nombre, b.Apellido, b.Documento, b.Nacimiento, b.Numero)
}

func sendLine(conn net.Conn, typ string, parts ...string) error {
	line := typ
	for _, p := range parts {
		line += "|" + p
	}
	line += "\n"
	return writeFrame(conn, []byte(line))
}

func readLine(conn net.Conn) (string, []string, error) {
	pkt, err := readFrame(conn)
	if err != nil {
		return "", nil, err
	}
	line := strings.TrimRight(string(pkt), "\r\n")
	fields := strings.Split(line, "|")
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("empty line")
	}
	return fields[0], fields[1:], nil
}

// ------- single bet -------
func SendBet(conn net.Conn, b Bet) error {
	if err := sendLine(conn, "BET", encodeBetLine(b)); err != nil {
		return err
	}
	typ, parts, err := readLine(conn)
	if err != nil {
		return err
	}
	if typ != "ACK" {
		return fmt.Errorf("unexpected reply: %s", typ)
	}
	if len(parts) >= 1 && parts[0] == "OK" {
		return nil
	}
	reason := ""
	if len(parts) >= 2 {
		reason = parts[1]
	}
	return fmt.Errorf("server NACK: %s", reason)
}

// ------- batch (auto split by size if needed) -------
const maxPayload = 8 * 1024

// Sends one batch header + lines (no auto-splitting)
func sendRawBatch(conn net.Conn, agencyID string, bets []Bet) error {
	header := fmt.Sprintf("BATCH|%s|%d\n", agencyID, len(bets))
	body := header
	for _, b := range bets {
		body += encodeBetLine(b) + "\n"
	}
	if len(body) > maxPayload {
		return fmt.Errorf("batch too large: %d", len(body))
	}
	if err := writeFrame(conn, []byte(body)); err != nil {
		return err
	}
	typ, parts, err := readLine(conn)
	if err != nil {
		return err
	}
	if typ != "ACK_BATCH" {
		return fmt.Errorf("unexpected reply: %s", typ)
	}
	if len(parts) >= 1 && parts[0] == "OK" {
		return nil
	}
	reason := ""
	if len(parts) >= 3 {
		reason = parts[2]
	}
	return fmt.Errorf("server NACK batch: %s", reason)
}

// Split bets into multiple frames (<=8KB each) and send sequentially.
func SendBatches(conn net.Conn, agencyID string, bets []Bet) error {
	i := 0
	for i < len(bets) {
		// binary search largest sub-slice starting at i that fits
		lo, hi := i+1, len(bets)+1
		best := -1
		for lo < hi {
			mid := (lo + hi) / 2
			header := fmt.Sprintf("BATCH|%s|%d\n", agencyID, mid-i)
			body := header
			for _, b := range bets[i:mid] {
				body += encodeBetLine(b) + "\n"
			}
			if len(body) <= maxPayload {
				best = mid
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if best == -1 {
			return fmt.Errorf("cannot fit bet at index %d", i)
		}
		if err := sendRawBatch(conn, agencyID, bets[i:best]); err != nil {
			return err
		}
		i = best
	}
	return nil
}
