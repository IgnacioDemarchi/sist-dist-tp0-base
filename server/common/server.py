import socket
import logging
import os
import threading
from common.comm import recv_line, send_line
from common.utils import Bet, store_bets, load_bets, has_won
class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self._server_socket.settimeout(0.5)  # periodic wake to check stop
        self._stopping = False    
        self._done_agencies = set()
        self._draw_completed = False
        self._winners_by_agency = {}
        self._expected_agencies_env = None
        val = os.getenv("CLIENT_AMOUNT")
        if val and val.strip():
            try:
                self._expected_agencies_env = int(val)
            except Exception:
                pass

        self._expected_agencies = self._expected_agencies_env  # may be None
        logging.debug(
            f"action: draw_config | result: success | expected_agencies: {self._expected_agencies or 'dynamic'}"
        )
        self._lock = threading.RLock()
        self._workers = set()
        self._max_workers = 64

    def run(self):
        while not self._stopping:
            try:
                client_sock = self.__accept_new_connection()
            except socket.timeout:
                # harvest finished workers to prevent set growth
                self._reap_workers()
                continue
            except OSError:
                break

            self._reap_workers()
            if len(self._workers) >= self._max_workers:
                try:
                    client_sock.close()
                except OSError:
                    pass
                continue

            t = threading.Thread(
                target=self.__handle_client_connection,
                args=(client_sock,),
                daemon=True,
            )
            with self._lock:
                self._workers.add(t)
            t.start()

    def _reap_workers(self):
        # Remove finished threads from the set
        with self._lock:
            dead = {t for t in self._workers if not t.is_alive()}
            if dead:
                self._workers.difference_update(dead)

    def __handle_client_connection(self, client_sock):
        try:
            while True:
                try:
                    line = recv_line(client_sock)
                except EOFError:
                    break
                parts = line.split("|")
                typ, rest = parts[0], parts[1:]

                if typ == "DONE":
                    agency = rest[0] if rest else "0"
                    self._done_agencies.add(agency)
                    self._perform_draw_if_ready()
                    send_line(client_sock, "ACK_DONE|OK")
                    continue

                if typ == "GET_WINNERS":
                    agency = rest[0] if rest else "0"
                    if not self._draw_completed:
                        send_line(client_sock, "WINNERS|ERR|not_ready")
                        continue
                    dnis = self._winners_by_agency.get(agency, [])
                    payload = "WINNERS|OK|" + ",".join(map(str, dnis))
                    send_line(client_sock, payload)
                    continue

                if typ == "BATCH":
                    # rest: [agency, count]
                    if len(rest) < 2:
                        send_line(client_sock, "ACK_BATCH|ERR|0|bad_header")
                        continue
                    agency = rest[0]
                    try:
                        count = int(rest[1])
                    except Exception:
                        send_line(client_sock, "ACK_BATCH|ERR|0|bad_count")
                        continue

                    bets = []
                    ok = True
                    err = ""
                    for _ in range(count):
                        try:
                            betline = recv_line(client_sock)
                        except EOFError:
                            ok = False
                            err = "missing_rows"
                            break
                        f = betline.split("|")
                        if len(f) < 5:
                            ok = False
                            err = "bad_row"
                            break
                        nombre, apellido, documento, nacimiento, numero = f[:5]
                        try:
                            b = Bet(agency, nombre, apellido, documento, nacimiento, numero)
                            bets.append(b)
                        except Exception as e:
                            ok = False
                            err = str(e)
                            break

                    if ok:
                        try:
                            store_bets(bets)
                            logging.info(f"action: apuesta_recibida | result: success | cantidad: {len(bets)}")
                            send_line(client_sock, f"ACK_BATCH|OK|{len(bets)}")
                        except Exception as e:
                            logging.error(f"action: apuesta_recibida | result: fail | cantidad: {len(bets)} | error: {e}")
                            send_line(client_sock, f"ACK_BATCH|ERR|{len(bets)}|persist_fail")
                    else:
                        logging.error(f"action: apuesta_recibida | result: fail | cantidad: {count} | error: {err}")
                        send_line(client_sock, f"ACK_BATCH|ERR|{count}|{err}")
                    continue

                if typ == "BET":
                    if len(rest) < 1:
                        send_line(client_sock, "ACK|ERR|bad_row")
                        continue
                    f = rest[0].split("|")
                    if len(f) < 6:
                        send_line(client_sock, "ACK|ERR|bad_row")
                        continue
                    agency, nombre, apellido, documento, nacimiento, numero = f[:6]
                    try:
                        b = Bet(agency, nombre, apellido, documento, nacimiento, numero)
                        store_bets([b])
                        logging.info(f"action: apuesta_almacenada | result: success | dni: {b.document} | numero: {b.number}")
                        send_line(client_sock, "ACK|OK")
                    except Exception as e:
                        logging.error(f"action: apuesta_almacenada | result: fail | dni: {documento} | numero: {numero} | error: {e}")
                        send_line(client_sock, "ACK|ERR|persist_fail")
                    continue

                # unknown
                send_line(client_sock, "ACK|ERR|unknown")
        except OSError as e:
            logging.error(f"action: client_handler | result: fail | error: {e}")
        finally:
            try:
                client_sock.close()
            except OSError:
                pass
            
    def __accept_new_connection(self):
        """
        Accept new connections

        Function blocks until a connection to a client is made.
        Then connection created is printed and returned
        """

        # Connection arrived
        logging.info('action: accept_connections | result: in_progress')
        c, addr = self._server_socket.accept()
        logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
        return c

    def stop(self):
        self._stopping = True
        try:
            self._server_socket.close()
        except OSError:
            pass
        # Best effort join outside lock to avoid deadlocks
        workers = []
        with self._lock:
            workers = list(self._workers)
        for t in workers:
            t.join(timeout=2.0)
            
    def _perform_draw_if_ready(self):
        with self._lock:
            if self._draw_completed:
                return
            if not self._expected_agencies:
                return
            if len(self._done_agencies) < self._expected_agencies:
                return

            winners = {}
            try:
                # load_bets is NOT thread-safe → safe because we hold the lock
                for bet in load_bets():
                    if has_won(bet):
                        winners.setdefault(str(bet.agency), []).append(str(bet.document))
            except Exception as e:
                logging.error(f"action: sorteo | result: fail | error: {e}")
                return

            self._winners_by_agency = winners
            self._draw_completed = True
        logging.info("action: sorteo | result: success")
