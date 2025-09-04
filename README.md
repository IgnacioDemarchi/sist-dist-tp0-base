### Ejercicio 5 – Quiniela: protocolo, serialización y persistencia

En este ejercicio se modificó la lógica del **cliente** y el **servidor** para modelar el caso de uso de una agencia de quiniela que registra apuestas en una central (Lotería Nacional). Se incorporó un **módulo de comunicación** con protocolo propio, serialización en texto delimitado y manejo robusto de sockets.

#### Ejecución

1) **Levantar servicios** (5 agencias/clientes como ejemplo):
    make docker-compose-up

2) **Configurar variables de entorno por cliente** (ejemplo para una agencia):

   #### Asignar por contenedor/servicio del cliente

    NOMBRE="Santiago Lionel"
    APELLIDO="Lorca"
    DOCUMENTO="30904465"
    NACIMIENTO="1999-03-17"
    NUMERO="7574"

   Cada cliente (agencia 1..5) debe tener sus propios valores. El cliente toma estos campos de `ENV` y los envía al servidor.

3) **Ver logs**:
    make docker-compose-logs

   - Cliente (éxito):
     action: apuesta_enviada | result: success | dni: 30904465 | numero: 7574

   - Servidor (persistencia ok):
     action: apuesta_almacenada | result: success | dni: 30904465 | numero: 7574

#### Detalles importantes de la solución

- **Variables de entorno (cliente)**  
  - `NOMBRE`, `APELLIDO`, `DOCUMENTO`, `NACIMIENTO` (`YYYY-MM-DD`), `NUMERO` (entero).  
  - El cliente arma la apuesta con la `AgencyID` (ID del cliente) y envía el mensaje.

- **Protocolo y serialización (módulo de comunicación)**  
  - **Framing** binario con **prefijo de longitud** de 4 bytes **big-endian** (`!I`).  
  - **Carga** serializada como línea delimitada por `|`:  
    - Solicitud (**BET**):  
      `BET|<agency_id>|<nombre>|<apellido>|<documento>|<nacimiento>|<numero>\n`
    - Respuesta (**ACK**):  
      `ACK|OK\n`  
      o en caso de error:  
      `ACK|ERR|<razón>\n`  
  - Funciones helper:  
    - **Python**: `send_line/recv_line` (evitan short read/write usando `sendall` y lecturas exactas).  
    - **Go**: `writeFrame/readFrame` con `io.ReadFull` + helpers de encoding/decoding de líneas.

- **Servidor (Python)**  
  - Acepta conexiones, recibe **BET**, mapea a `Bet(...)` y persiste con `store_bets([...])` (provista por la cátedra).  
  - Responde `ACK|OK` o `ACK|ERR|<razón>`.  
  - Manejo de errores y límites (p. ej., tamaño máximo de frame), logs de recepción y persistencia.  
  - Conserva el cierre *graceful* (SIGINT/SIGTERM) del ejercicio anterior.

- **Cliente (Go)**  
  - Lee `ENV`, construye `Bet`, invoca `SendBet`, valida **ACK** y loguea el resultado.  
  - Loop interrumpible y cierre *graceful* (canal `stopCh` + cierre de socket) ante `SIGTERM`.  
  - Logs de éxito/fracaso por apuesta.

- **Separación de responsabilidades**  
  - **Dominio**: `Bet` / `store_bets` (servidor).  
  - **Comunicación**: framing + (de)serialización en líneas.  
  - **Aplicación**: orquestación, validación, logs y control de ciclo de vida.

- **Manejo robusto de sockets**  
  - Prevención de **short read/write** (`sendall`/`io.ReadFull`/lecturas exactas).  
  - Validación de tamaños (rechazo de frames inválidos).  
  - Cierre seguro de conexiones en `finally/defer` y ante señales.
