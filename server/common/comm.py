import json
import socket
import struct

_MAX = 8 * 1024

def _recv_exact(sock: socket.socket, n: int) -> bytes:
    buf = bytearray()
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise EOFError("peer closed")
        buf.extend(chunk)
    return bytes(buf)

def recv_frame(sock: socket.socket) -> bytes:
    hdr = _recv_exact(sock, 4)
    (size,) = struct.unpack("!I", hdr)
    if size == 0 or size > _MAX:
        raise ValueError(f"invalid frame size {size}")
    return _recv_exact(sock, size)

def send_frame(sock: socket.socket, payload: bytes) -> None:
    if len(payload) > _MAX:
        raise ValueError(f"payload too large {len(payload)}")
    header = struct.pack("!I", len(payload))
    sock.sendall(header + payload)

def recv_line(sock) -> str:
    return recv_frame(sock).decode("utf-8").rstrip("\r\n")

def send_line(sock, line: str) -> None:
    if not line.endswith("\n"):
        line += "\n"
    send_frame(sock, line.encode("utf-8"))