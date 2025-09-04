import socket
import logging
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

    def run(self):
        """
        Accept new connections and serve each one inline (simple echo).
        Exits cleanly when stop() is called (socket closed / stop flag set).
        """
        while not self._stopping:
            try:
                client_sock = self.__accept_new_connection()
            except socket.timeout:
                continue                   # just loop back and re-check stop flag
            except OSError:
                # socket likely closed in stop()
                break
            self.__handle_client_connection(client_sock)

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
                    self._done_agencies.add(agency)
                    self._perform_draw_if_ready()
                    try:
                        send_json(client_sock, {"type": "ACK_DONE", "ok": True})
                    except Exception:
                        pass
                    continue

                # ----- GET_WINNERS -----
                if mtype == "GET_WINNERS":
                    agency = str(msg.get("agency_id", "0"))
                    if not self._draw_completed:
                        send_json(client_sock, {"type": "WINNERS", "ok": False, "error": "not_ready"})
                        continue
                    dnis = self._winners_by_agency.get(agency, [])
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

                        # persist atomically
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
            self._server_socket.close()  # unblocks accept()
        except OSError:
            pass
        
    def _perform_draw_if_ready(self):
        if self._draw_completed:
            return
        # Wait until we have the 5 agencies
        if len(self._done_agencies) < 5:
            return

        # Build winners per agency from persisted bets
        winners = {}
        try:
            for bet in load_bets():
                if has_won(bet):
                    winners.setdefault(str(bet.agency), []).append(str(bet.document))
        except Exception as e:
            logging.error(f"action: sorteo | result: fail | error: {e}")
            return

        self._winners_by_agency = winners
        self._draw_completed = True
        logging.info("action: sorteo | result: success")
