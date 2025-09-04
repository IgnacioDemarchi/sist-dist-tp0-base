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
        Accept BET messages, persist them, ACK, and keep going
        until client closes connection.
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

                if typ != "BET":
                    # Unknown or missing type → negative ACK
                    try:
                        send_line(client_sock, f"ACK|ERR|unknown message type")
                    except Exception as e:
                        logging.error(f"action: recv | result: fail | error: unknown message type {e}")
                    continue


                if len(rest) < 6:
                    send_line(client_sock, "ACK|ERR|bad_row")
                    continue
                agency, nombre, apellido, documento, nacimiento, numero = rest[:6]

                # Map fields from client payload to Bet(...)
                try:
                    b = Bet(agency, nombre, apellido, documento, nacimiento, numero)
                    store_bets([b])
                    logging.info(f"action: apuesta_almacenada | result: success | dni: {b.document} | numero: {b.number}")
                    send_line(client_sock, "ACK|OK")
                except Exception as e:
                    logging.error(f"action: apuesta_almacenada | result: fail | dni: {documento} | numero: {numero} | error: {e}")
                    send_line(client_sock, "ACK|ERR|persist_fail")
                continue

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