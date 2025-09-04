import socket
import logging
import os
import threading
from common.comm import recv_json, send_json 
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
        """
        Receive length-prefixed JSON frames.
        Accept BET messages, persist them, ACK, and keep going
        until client closes connection.
        """
        try:
            while True:
                try:
                    msg = recv_json(client_sock)
                except EOFError:
                    break  # client closed
                except Exception as e:
                    logging.error(f"action: recv | result: fail | error: {e}")
                    break

                mtype = msg.get("type")

                # ----- DONE -----
                if mtype == "DONE":
                    agency = str(msg.get("agency_id", "0"))
                    with self._lock:
                        self._done_agencies.add(agency)
                        if self._expected_agencies_env is None:
                            try:
                                ids = [int(a) for a in self._done_agencies if a.isdigit()]
                                if ids:
                                    self._expected_agencies = max(ids)
                            except Exception:
                                pass
                        logging.debug(
                            f"action: done_received | result: success | agency: {agency} | "
                            f"done_count: {len(self._done_agencies)} | expected: {self._expected_agencies}"
                        )
                        self._perform_draw_if_ready()

                    try:
                        send_json(client_sock, {"type": "ACK_DONE", "ok": True})
                    except Exception:
                        pass
                    continue
                    # ----- GET_WINNERS -----
                if mtype == "GET_WINNERS":
                    agency = str(msg.get("agency_id", "0"))
                    with self._lock:
                        if not self._draw_completed:
                            send_json(client_sock, {"type": "WINNERS", "ok": False, "error": "not_ready"})
                        else:
                            dnis = list(self._winners_by_agency.get(agency, []))
                            send_json(client_sock, {"type": "WINNERS", "ok": True, "dnis": dnis})
                    continue


                if mtype == "BATCH_BET":
                    agency = msg.get("agency_id", "0")
                    rows = msg.get("bets", [])
                    count = len(rows)
                    try:
                        bets = []
                        for b in rows:
                            bet = Bet(
                                agency=agency,
                                first_name=b.get("nombre", ""),
                                last_name=b.get("apellido", ""),
                                document=b.get("documento", ""),
                                birthdate=b.get("nacimiento", ""),
                                number=str(b.get("numero", 0)),
                            )
                            bets.append(bet)

                        # persist atomically and log under lock to keep count consistent
                        with self._lock:
                            store_bets(bets)
                        logging.info(f"action: apuesta_recibida | result: success | cantidad: {count}")
                        send_json(client_sock, {"type": "ACK_BATCH", "ok": True, "count": count})

                    except Exception as e:
                        logging.error(f"action: apuesta_recibida | result: fail | cantidad: {count} | error: {e}")
                        send_json(client_sock, {"type": "ACK_BATCH", "ok": False, "count": count, "error": str(e)})
                    continue


                if mtype != "BET":
                    # Unknown or missing type → negative ACK
                    try:
                        send_json(client_sock, {"type": "ACK", "ok": False, "error": "unknown message type"})
                    except Exception:
                        pass
                    continue

                # Map fields from client payload to Bet(...)
                try:
                    bet = Bet(
                        agency=msg.get("agency_id", "0"),
                        first_name=msg.get("nombre", ""),
                        last_name=msg.get("apellido", ""),
                        document=msg.get("documento", ""),
                        birthdate=msg.get("nacimiento", "1970-01-01"),
                        number=str(msg.get("numero", 0)),
                    )
                    with self._lock:
                        store_bets([bet])
                    logging.info(
                        f"action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}"
                    )

                    send_json(client_sock, {"type": "ACK", "ok": True})

                except Exception as e:
                    logging.error(
                        f"action: apuesta_almacenada | result: fail | dni: {msg.get('documento','')} "
                        f"| numero: {msg.get('numero',0)} | error: {e}"
                    )
                    try:
                        send_json(client_sock, {"type": "ACK", "ok": False, "error": str(e)})
                    except Exception:
                        pass

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
