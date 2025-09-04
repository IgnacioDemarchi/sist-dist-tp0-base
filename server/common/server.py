import socket
import logging
from common.comm import recv_line, send_line
from common.utils import Bet, store_bets
class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self._server_socket.settimeout(0.5)  # periodic wake to check stop
        self._stopping = False    

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
        Receive length-prefixed text frames (UTF-8 lines).
        Supports:
        - BET|agency|nombre|apellido|documento|nacimiento|numero
        - BATCH|agency|count   (followed by <count> BET lines)
        """
        try:
            while True:
                try:
                    line = recv_line(client_sock)
                except EOFError:
                    break  # client closed
                except Exception as e:
                    logging.error(f"action: recv | result: fail | error: {e}")
                    break

                parts = line.split("|")
                typ, rest = parts[0], parts[1:]

                # ---------- BATCH header ----------
                if typ == "BATCH":
                    if len(rest) < 2:
                        try:
                            send_line(client_sock, "ACK_BATCH|ERR|bad_header")
                        except Exception:
                            pass
                        continue
                    agency = rest[0]
                    try:
                        count = int(rest[1])
                        if count < 0:
                            raise ValueError("negative")
                    except Exception:
                        try:
                            send_line(client_sock, "ACK_BATCH|ERR|bad_count")
                        except Exception:
                            pass
                        continue

                    bets = []
                    ok = True
                    err_reason = ""
                    for _ in range(count):
                        try:
                            bet_line = recv_line(client_sock)
                        except Exception as e:
                            ok = False
                            err_reason = f"recv_bet: {e}"
                            break
                        f = bet_line.split("|")
                        if len(f) < 7 or f[0] != "BET":
                            ok = False
                            err_reason = "bad_bet_line"
                            break
                        _, a, nombre, apellido, documento, nacimiento, numero = f[:7]
                        try:
                            b = Bet(a, nombre, apellido, documento, nacimiento, numero)
                            bets.append(b)
                        except Exception as e:
                            ok = False
                            err_reason = f"parse_bet: {e}"
                            break

                    if not ok:
                        logging.error(f"action: apuesta_recibida | result: fail | cantidad: {count} | error: {err_reason}")
                        try:
                            send_line(client_sock, f"ACK_BATCH|ERR|{err_reason}")
                        except Exception:
                            pass
                        continue

                    try:
                        store_bets(bets)
                        logging.info(f"action: apuesta_recibida | result: success | cantidad: {count}")
                        send_line(client_sock, f"ACK_BATCH|OK|{count}")
                    except Exception as e:
                        logging.error(f"action: apuesta_recibida | result: fail | cantidad: {count} | error: {e}")
                        try:
                            send_line(client_sock, f"ACK_BATCH|ERR|persist_fail")
                        except Exception:
                            pass
                    continue

                # ---------- Single BET ----------
                if typ != "BET":
                    # Unknown or missing type → negative ACK
                    try:
                        send_line(client_sock, "ACK|ERR|unknown message type")
                    except Exception as e:
                        logging.error(f"action: recv | result: fail | error: unknown message type {e}")
                    continue

                if len(rest) < 6:
                    send_line(client_sock, "ACK|ERR|bad_row")
                    continue

                agency, nombre, apellido, documento, nacimiento, numero = rest[:6]

                try:
                    b = Bet(agency, nombre, apellido, documento, nacimiento, numero)
                    store_bets([b])
                    logging.info(
                        f"action: apuesta_almacenada | result: success | dni: {b.document} | numero: {b.number}"
                    )
                    send_line(client_sock, "ACK|OK")
                except Exception as e:
                    logging.error(
                        f"action: apuesta_almacenada | result: fail | dni: {documento} | numero: {numero} | error: {e}"
                    )
                    send_line(client_sock, "ACK|ERR|persist_fail")

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