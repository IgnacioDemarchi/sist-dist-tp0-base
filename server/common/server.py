import socket
import logging


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
        Read a full line (until '\n') from the client and echo it back.
        Always send all bytes back (no short-writes).
        """
        try:
            # robust receive: keep reading until newline or EOF
            data = bytearray()
            while b'\n' not in data:
                chunk = client_sock.recv(4096)
                if not chunk:
                    break
                data.extend(chunk)

            msg = bytes(data).rstrip(b'\r\n').decode('utf-8', errors='replace')
            addr = client_sock.getpeername()
            logging.info(f'action: receive_message | result: success | ip: {addr[0]} | msg: {msg}')

            # robust send: sendall avoids short-writes
            client_sock.sendall((msg + "\n").encode('utf-8'))
        except OSError as e:
            logging.error(f"action: receive_message | result: fail | error: {e}")
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