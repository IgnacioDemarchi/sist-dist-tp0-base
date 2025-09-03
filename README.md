### Ejercicio 5 – Quiniela: protocolo, serialización y persistencia

En este ejercicio se modificó la lógica del **cliente** y el **servidor** para modelar el caso de uso de una agencia de quiniela que registra apuestas en una central (Lotería Nacional). Se incorporó un **módulo de comunicación** con protocolo propio, serialización JSON y manejo robusto de sockets.

#### Ejecución

1) **Levantar servicios** (5 agencias/clientes como ejemplo):
```bash
make docker-compose-up
```

2) **Configurar variables de entorno por cliente** (ejemplo para una agencia):
```bash
# Asignar por contenedor/servicio del cliente
NOMBRE="Santiago Lionel"
APELLIDO="Lorca"
DOCUMENTO="30904465"
NACIMIENTO="1999-03-17"
NUMERO="7574"
```
> Cada cliente (agencia 1..5) debe tener sus propios valores. El cliente toma estos campos de `ENV` y los envía al servidor.

3) **Ver logs**:
```bash
make docker-compose-logs
```

- Cliente (éxito):
```
action: apuesta_enviada | result: success | dni: 30904465 | numero: 7574
```
- Servidor (persistencia ok):
```
action: apuesta_almacenada | result: success | dni: 30904465 | numero: 7574
```

#### Detalles importantes de la solución

- **Variables de entorno (cliente)**  
  - `NOMBRE`, `APELLIDO`, `DOCUMENTO`, `NACIMIENTO` (`YYYY-MM-DD`), `NUMERO` (entero).  
  - El cliente arma la apuesta con la `AgencyID` (ID del cliente) y envía el mensaje.

- **Protocolo y serialización (módulo de comunicación)**
  - **Framing** binario con **prefijo de longitud** de 4 bytes **big-endian** (`!I`).
  - **Carga** serializada en **JSON**:
    - Solicitud (**BET**):
      ```json
      {
        "type": "BET",
        "agency_id": "1",
        "nombre": "Santiago Lionel",
        "apellido": "Lorca",
        "documento": "30904465",
        "nacimiento": "1999-03-17",
        "numero": 7574
      }
      ```
    - Respuesta (**ACK**):
      ```json
      { "type": "ACK", "ok": true }
      ```
  - Funciones helper:
    - **Python**: `send_frame/recv_frame`, `send_json/recv_json` (evitan short read/write usando `sendall` y lecturas exactas).
    - **Go**: `writeFrame/readFrame` con `io.ReadFull` + `json.Marshal/Unmarshal`.

- **Servidor (Python)**
  - Acepta conexiones, recibe **BET**, mapea a `Bet(...)` y persiste con `store_bets([...])` (provista por la cátedra).  
  - Responde **ACK { ok: true/false }**.  
  - Manejo de errores y límites (p. ej., tamaño máximo de frame), logs de recepción y persistencia.  
  - Conserva el cierre *graceful* (SIGINT/SIGTERM) del ejercicio anterior.

- **Cliente (Go)**
  - Lee `ENV`, construye `Bet`, invoca `SendBet`, valida **ACK** y loguea el resultado.  
  - Loop interrumpible y cierre *graceful* (canal `stopCh` + cierre de socket) ante `SIGTERM`.  
  - Logs de éxito/fracaso por apuesta.

- **Separación de responsabilidades**
  - **Dominio**: `Bet` / `store_bets` (servidor).  
  - **Comunicación**: framing + (de)serialización JSON y envío/recepción.  
  - **Aplicación**: orquestación, validación, logs y control de ciclo de vida.

- **Manejo robusto de sockets**
  - Prevención de **short read/write** (`sendall`/`io.ReadFull`/lecturas exactas).  
  - Validación de tamaños (rechazo de frames inválidos).  
  - Cierre seguro de conexiones en `finally/defer` y ante señales.

